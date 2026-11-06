package features

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

type hardlinkWorld struct {
	path, alias, closed, command string
	initial, linked, outside     hardlinkEntry
	before, hidden               map[string]hardlinkEntry
	identity                     map[string]any
	results                      []doctorResult
	journals                     [][]doctorRequest
	connections                  []int64
	reached                      bool
	ownedErr                     error
	native                       []hardlinkNativeResult
	resources                    map[string]string
}

type hardlinkEntry struct {
	Data                           string
	Mode                           os.FileMode
	UID, GID, Device, Inode, Links string
	Modified                       int64
}

func hardlinkObserve(path string) (hardlinkEntry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return hardlinkEntry{}, err
	}
	e := hardlinkEntry{Mode: info.Mode(), Modified: info.ModTime().UnixNano()}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	for name, target := range map[string]*string{"Uid": &e.UID, "Gid": &e.GID, "Dev": &e.Device, "Ino": &e.Inode, "Nlink": &e.Links} {
		if !stat.IsValid() || stat.Kind() != reflect.Struct || !stat.FieldByName(name).IsValid() {
			return e, fmt.Errorf("missing fixture metadata %s", name)
		}
		*target = fmt.Sprint(stat.FieldByName(name).Interface())
	}
	if info.Mode().IsRegular() {
		data, err := os.ReadFile(path)
		if err != nil {
			return e, err
		}
		e.Data = string(data)
	} else if info.Mode()&os.ModeSymlink != 0 {
		e.Data, err = os.Readlink(path)
	}
	return e, err
}

// Skip only the explicitly inaccessible fixture directory. No symlink or
// special file is opened; all other entries, including new ones, are recorded.
func hardlinkTree(root, closed string) (map[string]hardlinkEntry, error) {
	tree := map[string]hardlinkEntry{}
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		e, err := hardlinkObserve(path)
		if err != nil {
			return err
		}
		tree[path] = e
		if path == closed {
			return filepath.SkipDir
		}
		return nil
	})
	return tree, err
}

func (w *world) hardlinkFixture() (*hardlinkWorld, error) {
	if os.Geteuid() == 0 {
		return nil, fmt.Errorf("hard-link fixture requires non-root effective UID")
	}
	if w.hardlinks == nil {
		identity, err := doctorCheckoutIdentity(w.doctor.checkout)
		if err != nil {
			return nil, err
		}
		w.hardlinks = &hardlinkWorld{identity: identity}
	}
	return w.hardlinks, nil
}

func (w *world) hardlinkFile(path, mode string) error {
	h, err := w.hardlinkFixture()
	if err != nil {
		return err
	}
	perm, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return err
	}
	h.path = filepath.Join(w.doctor.checkout, path)
	if _, err := os.Lstat(h.path); os.IsNotExist(err) {
		if err := writeManifestFixture(h.path, []byte("slice-20 original synthetic data\n"), os.FileMode(perm)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := os.Chmod(h.path, os.FileMode(perm)); err != nil {
		return err
	}
	if strings.HasPrefix(path, "ignored/") {
		if err := writeManifestFixture(filepath.Join(w.doctor.checkout, ".gitignore"), []byte("ignored/\n"), 0600); err != nil {
			return err
		}
	}
	h.initial, err = hardlinkObserve(h.path)
	return err
}

func (w *world) hardlinkAlias(kind string) error {
	h := w.hardlinks
	parent := filepath.Join(w.dir, "slice-20-outside")
	if kind == "inside" {
		parent = filepath.Join(w.doctor.checkout, "slice-20-aliases")
	} else if kind != "outside" {
		return fmt.Errorf("unknown alias kind %q", kind)
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	h.alias = filepath.Join(parent, "alias")
	if err := os.Link(h.path, h.alias); err != nil {
		return err
	}
	var err error
	h.linked, err = hardlinkObserve(h.path)
	if err != nil {
		return err
	}
	h.outside, err = hardlinkObserve(h.alias)
	return err
}

func (w *world) hardlinkLink(path, kind, mode string) error {
	if err := w.hardlinkFile(path, mode); err != nil {
		return err
	}
	return w.hardlinkAlias(kind)
}

func (w *world) hardlinkCheckpoint() error {
	h, err := w.hardlinkFixture()
	if err != nil {
		return err
	}
	if w.approval != nil {
		w.runFixture()
	}
	h.before, err = hardlinkTree(w.dir, h.closed)
	return err
}

func (w *world) hardlinkHidden() error {
	if err := w.hardlinkLink(".slice-20-closed/nested/file", "outside", "0600"); err != nil {
		return err
	}
	h := w.hardlinks
	h.closed = filepath.Join(w.doctor.checkout, ".slice-20-closed")
	var err error
	h.hidden, err = hardlinkTree(h.closed, "")
	if err != nil {
		return err
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.Chmod(h.closed, 0700) })
	if err := os.Chmod(h.closed, 0000); err != nil {
		return err
	}
	return w.hardlinkCheckpoint()
}

func hardlinkEqual(want, got map[string]hardlinkEntry) error {
	if len(want) != len(got) {
		return fmt.Errorf("fixture entry count changed: %d to %d", len(want), len(got))
	}
	for path, entry := range want {
		if got[path] != entry {
			return fmt.Errorf("fixture entry changed: %s\nwant %+v\ngot %+v", path, entry, got[path])
		}
	}
	return nil
}

func (w *world) hardlinkUnchanged() error {
	got, err := hardlinkTree(w.dir, w.hardlinks.closed)
	if err != nil {
		return err
	}
	return hardlinkEqual(w.hardlinks.before, got)
}

func (w *world) hardlinkHostUnchanged() error {
	got, err := hardlinkTree(w.doctor.host, "")
	if err != nil {
		return err
	}
	want := map[string]hardlinkEntry{}
	for path, entry := range w.hardlinks.before {
		if path == w.doctor.host || strings.HasPrefix(path, w.doctor.host+string(os.PathSeparator)) {
			want[path] = entry
		}
	}
	return hardlinkEqual(want, got)
}

func (w *world) hardlinkHiddenUnchanged() error {
	h := w.hardlinks
	// Observe refusal and inaccessible metadata BEFORE restoring harness access.
	if err := w.hardlinkUnchanged(); err != nil {
		return err
	}
	e, err := hardlinkObserve(h.closed)
	if err != nil {
		return err
	}
	if e.Mode.Perm() != 0 {
		return fmt.Errorf("product repaired inaccessible subtree")
	}
	if err := os.Chmod(h.closed, 0700); err != nil {
		return err
	}
	defer func() { _ = os.Chmod(h.closed, 0000) }()
	got, err := hardlinkTree(h.closed, "")
	if err != nil {
		return err
	}
	return hardlinkEqual(h.hidden, got)
}

func (w *world) hardlinkShared() error {
	h := w.hardlinks
	if h.linked.Inode == "" || h.linked.Device != h.outside.Device || h.linked.Inode != h.outside.Inode || h.linked.Links != "2" || h.outside.Links != "2" {
		return fmt.Errorf("fixture names did not share a two-link inode")
	}
	return w.hardlinkIdentity()
}

func (w *world) hardlinkIdentity() error {
	got, err := doctorCheckoutIdentity(w.doctor.checkout)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(got, w.hardlinks.identity) {
		return fmt.Errorf("fixture changed checkout or Git identity")
	}
	return nil
}

func (w *world) hardlinkCopy() error {
	h := w.hardlinks
	if err := os.Remove(h.path); err != nil {
		return err
	}
	// Copy the initial bytes, including after the deliberately unsafe red run.
	return writeManifestFixture(h.path, []byte(h.initial.Data), h.initial.Mode.Perm())
}

func (w *world) hardlinkCopyIndependent() error {
	h := w.hardlinks
	got, err := hardlinkObserve(h.path)
	if err != nil {
		return err
	}
	if got.Data != h.initial.Data || got.Mode != h.initial.Mode || got.Links != "1" || (got.Device == h.outside.Device && got.Inode == h.outside.Inode) {
		return fmt.Errorf("copy is not independent with original bytes/mode: %+v", got)
	}
	return w.hardlinkIdentity()
}

func (w *world) hardlinkOutsideAfterCopy() error {
	got, err := hardlinkObserve(w.hardlinks.alias)
	if err != nil {
		return err
	}
	want := w.hardlinks.outside
	want.Links = "1"
	if got != want {
		return fmt.Errorf("outside file changed beyond removed alias")
	}
	return w.hardlinkHostUnchanged()
}
