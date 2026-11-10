package features

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the real preservation chain using disposable files only: no native
// fixture, product process, approval command or engine connection is involved.
func TestRecoveryProtectedHeartbeatRename(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native preservation observations target Linux")
	}
	t.Run("unchanged", func(t *testing.T) {
		w := recoveryPreservationFixture(t)
		before := recoveryCapture(t, w)
		recoveryRequireSentinels(t, w, before)
		recoveryRequireSame(t, before, recoveryCapture(t, w))
	})
	for _, cached := range []bool{false, true} {
		name := "rename_after_listing"
		if cached {
			name = "rename_with_cached_info"
		}
		t.Run(name, func(t *testing.T) {
			w := recoveryPreservationFixture(t)
			before := recoveryCapture(t, w)
			recoveryRequireSentinels(t, w, before)
			published := filepath.Join(w.doctor.checkout, "slice-11-heartbeat")
			staging := published + ".next"
			renamed := 0
			walk := recoveryChangingWalk(t, staging, cached, &renamed, func() error {
				return os.Rename(staging, published)
			})
			after, err := w.recoveryProtectedWithWalk(walk)
			if renamed != 1 {
				t.Fatalf("rename count = %d, want exactly one", renamed)
			}
			data, readErr := os.ReadFile(published)
			if readErr != nil || string(data) != "2" {
				t.Fatalf("heartbeat did not advance from 1 to 2: %q, %v", data, readErr)
			}
			t.Log("performed exactly one rename; published heartbeat advanced from 1 to 2")
			if err != nil {
				t.Fatalf("capture after heartbeat rename: %v", err)
			}
			recoveryRequireSentinels(t, w, after)
			recoveryRequireSame(t, before, after)
		})
	}
	t.Run("excluded_info_calls", func(t *testing.T) {
		w := recoveryPreservationFixture(t)
		before := recoveryCapture(t, w)
		delivered, inspected := map[string]int{}, map[string]int{}
		published := filepath.Join(w.doctor.checkout, "slice-11-heartbeat")
		walk := func(root string, visit fs.WalkDirFunc) error {
			return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err == nil && (path == published || path == published+".next") {
					delivered[path]++
					original := entry
					entry = recoveryObservedEntry{DirEntry: original, info: func() (fs.FileInfo, error) {
						inspected[path]++
						return original.Info()
					}}
				}
				return visit(path, entry, err)
			})
		}
		after, err := w.recoveryProtectedWithWalk(walk)
		for _, path := range []string{published, published + ".next"} {
			if delivered[path] != 1 || inspected[path] != 0 {
				t.Errorf("%s: delivered %d times, collector Info calls %d; want 1 and 0", path, delivered[path], inspected[path])
			}
		}
		if err != nil {
			t.Fatal("capture with counted metadata calls:", err)
		}
		recoveryRequireSentinels(t, w, after)
		recoveryRequireSame(t, before, after)
	})
}

func TestRecoveryProtectedSnapshotBoundaries(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native preservation observations target Linux")
	}
	t.Run("healthy", func(t *testing.T) {
		w := recoveryPreservationFixture(t)
		before := recoveryCapture(t, w)
		recoveryRequireSentinels(t, w, before)
		recoveryRequireSame(t, before, recoveryCapture(t, w))
	})
	t.Run("exact_paths", recoveryExactPathControls)
	t.Run("excluded_directories", recoveryDirectoryControls)
	t.Run("unrelated_changes", recoveryUnrelatedControls)
	t.Run("strict_defaults", recoveryDefaultControls)
	t.Run("missing_entries", recoveryMissingControls)
	t.Run("symlink", recoveryLinkControls)
}

func recoveryExactPathControls(t *testing.T) {
	for _, relative := range []string{
		"checkout/unrelated.next",
		"checkout/nested/slice-11-heartbeat", "checkout/nested/slice-11-heartbeat.next",
		"host/slice-11-heartbeat", "host/slice-11-heartbeat.next",
		"checkout/slice-11-heartbeat.keep", "checkout/slice-11-heartbeat.next.keep",
	} {
		t.Run(relative, func(t *testing.T) {
			w := recoveryPreservationFixture(t)
			path := filepath.Join(filepath.Dir(w.doctor.checkout), filepath.FromSlash(relative))
			recoveryWrite(t, path, "retained neighbour")
			before := recoveryCapture(t, w)
			recoveryRequireSentinels(t, w, before)
			recoveryRequireEntry(t, before, path, 0600, "retained neighbour")
			recoveryWrite(t, path, "changed neighbour")
			after := recoveryCapture(t, w)
			recoveryRequireEntry(t, after, path, 0600, "changed neighbour")
			recoveryRequireChanged(t, before, after, path)
		})
	}
}

func recoveryDirectoryControls(t *testing.T) {
	for _, name := range []string{"slice-11-heartbeat", "slice-11-heartbeat.next"} {
		for _, changed := range []string{"child", "slice-11-heartbeat.keep", "slice-11-heartbeat.next.keep"} {
			t.Run(name+"/"+changed, func(t *testing.T) {
				w := recoveryPreservationFixture(t)
				directory := filepath.Join(w.doctor.checkout, name)
				if err := os.Remove(directory); err != nil {
					t.Fatal(err)
				}
				child := filepath.Join(directory, "child")
				paths := map[string]string{
					"child":                        child,
					"slice-11-heartbeat.keep":      filepath.Join(w.doctor.checkout, "slice-11-heartbeat.keep"),
					"slice-11-heartbeat.next.keep": filepath.Join(w.doctor.checkout, "slice-11-heartbeat.next.keep"),
				}
				for _, path := range paths {
					recoveryWrite(t, path, "retained child or neighbour")
				}
				before := recoveryCapture(t, w)
				recoveryRequireSentinels(t, w, before)
				for _, path := range paths {
					recoveryRequireEntry(t, before, path, 0600, "retained child or neighbour")
				}
				path := paths[changed]
				recoveryWrite(t, path, "independently changed")
				after := recoveryCapture(t, w)
				recoveryRequireEntry(t, after, path, 0600, "independently changed")
				recoveryRequireChanged(t, before, after, path)
			})
		}
	}
}

func recoveryUnrelatedControls(t *testing.T) {
	for _, location := range []string{"checkout", "host", "approval"} {
		for _, change := range []string{"addition", "removal", "content", "mode"} {
			t.Run(location+"/"+change, func(t *testing.T) {
				w := recoveryPreservationFixture(t)
				path := filepath.Join(w.doctor.checkout, "sentinel")
				switch location {
				case "host":
					path = filepath.Join(w.doctor.host, "sentinel")
				case "approval":
					path = recoveryApprovalSentinel(w)
				}
				before := recoveryCapture(t, w)
				recoveryRequireSentinels(t, w, before)
				var err error
				switch change {
				case "addition":
					path += ".added"
					if _, exists := before[path]; exists {
						t.Fatal("addition already exists in baseline")
					}
					err = os.WriteFile(path, []byte("added"), 0600)
				case "removal":
					err = os.Remove(path)
				case "content":
					err = os.WriteFile(path, []byte("changed"), 0600)
				case "mode":
					err = os.Chmod(path, 0640)
				}
				if err != nil {
					t.Fatal(err)
				}
				after := recoveryCapture(t, w)
				switch change {
				case "addition":
					recoveryRequireEntry(t, after, path, 0600, "added")
				case "removal":
					if _, exists := after[path]; exists {
						t.Fatal("removed entry remains in capture")
					}
				case "content":
					recoveryRequireEntry(t, after, path, 0600, "changed")
				case "mode":
					contents := "original"
					if location == "approval" {
						contents = "retained approval sentinel"
					}
					recoveryRequireEntry(t, after, path, 0640, contents)
				}
				recoveryRequireChanged(t, before, after, path)
			})
		}
	}
}

func recoveryDefaultControls(t *testing.T) {
	for _, collector := range []string{"doctorTree", "offlineProtected"} {
		for _, name := range []string{"slice-11-heartbeat", "slice-11-heartbeat.next"} {
			t.Run(collector+"/"+name, func(t *testing.T) {
				w := recoveryPreservationFixture(t)
				capture := func() (map[string]string, error) { return doctorTree(w.doctor.checkout) }
				if collector == "offlineProtected" {
					capture = w.offlineProtected
				}
				before, err := capture()
				if err != nil {
					t.Fatal(err)
				}
				published := filepath.Join(w.doctor.checkout, "slice-11-heartbeat")
				recoveryRequireEntry(t, before, published, 0600, "1")
				recoveryRequireEntry(t, before, published+".next", 0600, "2")
				path := filepath.Join(w.doctor.checkout, name)
				recoveryWrite(t, path, "3")
				after, err := capture()
				if err != nil {
					t.Fatal(err)
				}
				recoveryRequireEntry(t, after, path, 0600, "3")
				recoveryRequireChanged(t, before, after, path)
			})
		}
	}
}

func recoveryMissingControls(t *testing.T) {
	for _, cached := range []bool{false, true} {
		op := "lstat"
		if cached {
			op = "open"
		}
		t.Run(op, func(t *testing.T) {
			w := recoveryPreservationFixture(t)
			path := filepath.Join(w.doctor.checkout, "unrelated.next")
			recoveryWrite(t, path, "must remain audited")
			recoveryRequireEntry(t, recoveryCapture(t, w), path, 0600, "must remain audited")
			removed := 0
			walk := recoveryChangingWalk(t, path, cached, &removed, func() error { return os.Remove(path) })
			_, err := w.recoveryProtectedWithWalk(walk)
			if removed != 1 {
				t.Fatalf("removal count = %d, want exactly one", removed)
			}
			recoveryRequireMissing(t, err, path, op)
		})
	}
}

func recoveryLinkControls(t *testing.T) {
	w := recoveryPreservationFixture(t)
	outside, other := t.TempDir(), t.TempDir()
	child := filepath.Join(outside, "child")
	recoveryWrite(t, child, "outside original")
	link := filepath.Join(w.doctor.checkout, "retained-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	before := recoveryCapture(t, w)
	recoveryRequireSentinels(t, w, before)
	recoveryRequireEntry(t, before, link, os.ModeSymlink|0777, outside)
	recoveryRequireNoLinkChildren(t, before, link, outside)
	recoveryWrite(t, child, "outside changed")
	afterChild := recoveryCapture(t, w)
	recoveryRequireSame(t, before, afterChild)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	afterTarget := recoveryCapture(t, w)
	recoveryRequireEntry(t, afterTarget, link, os.ModeSymlink|0777, other)
	recoveryRequireNoLinkChildren(t, afterTarget, link, other)
	recoveryRequireChanged(t, before, afterTarget, link)
	removed := 0
	walk := recoveryChangingWalk(t, link, true, &removed, func() error { return os.Remove(link) })
	_, err := w.recoveryProtectedWithWalk(walk)
	if removed != 1 {
		t.Fatalf("link removal count = %d, want exactly one", removed)
	}
	recoveryRequireMissing(t, err, link, "readlink")
}

func recoveryPreservationFixture(t *testing.T) *world {
	t.Helper()
	w, _, _ := offlineRegistryFixture(t)
	recoveryWrite(t, filepath.Join(w.doctor.checkout, "slice-11-heartbeat"), "1")
	recoveryWrite(t, filepath.Join(w.doctor.checkout, "slice-11-heartbeat.next"), "2")
	recoveryWrite(t, recoveryApprovalSentinel(w), "retained approval sentinel")
	return w
}

func recoveryApprovalSentinel(w *world) string {
	return filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "approvals", "sentinel.json")
}

func recoveryWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := writeManifestFixture(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func recoveryCapture(t *testing.T, w *world) map[string]string {
	t.Helper()
	got, err := w.recoveryProtected()
	if err != nil {
		t.Fatal("capture protected snapshot:", err)
	}
	return got
}

func recoveryRequireSentinels(t *testing.T, w *world, got map[string]string) {
	t.Helper()
	for _, root := range []string{w.doctor.checkout, w.doctor.host} {
		recoveryRequireEntry(t, got, filepath.Join(root, "sentinel"), 0600, "original")
	}
	recoveryRequireEntry(t, got, recoveryApprovalSentinel(w), 0600, "retained approval sentinel")
	for _, name := range []string{"slice-11-heartbeat", "slice-11-heartbeat.next"} {
		if _, exists := got[filepath.Join(w.doctor.checkout, name)]; exists {
			t.Errorf("recovery captured excluded entry %s", name)
		}
	}
}

func recoveryRequireEntry(t *testing.T, got map[string]string, path string, mode fs.FileMode, contents string) {
	t.Helper()
	want := fmt.Sprintf("%v:%d:%s", mode, os.Geteuid(), contents)
	if value, exists := got[path]; !exists || value != want {
		t.Fatalf("snapshot[%s] = %q (present %t), want %q", path, value, exists, want)
	}
}

func recoveryRequireSame(t *testing.T, before, after map[string]string) {
	t.Helper()
	if !maps.Equal(before, after) {
		t.Fatal("protected snapshot changed")
	}
}

func recoveryRequireChanged(t *testing.T, before, after map[string]string, path string) {
	t.Helper()
	if maps.Equal(before, after) || before[path] == after[path] {
		t.Fatalf("protected snapshot failed to detect change to %s", path)
	}
}

func recoveryRequireNoLinkChildren(t *testing.T, got map[string]string, link, target string) {
	t.Helper()
	for path := range got {
		if strings.HasPrefix(path, link+string(os.PathSeparator)) || path == target || strings.HasPrefix(path, target+string(os.PathSeparator)) {
			t.Fatalf("snapshot followed symlink: %s", path)
		}
	}
}

func recoveryRequireMissing(t *testing.T, err error, path, op string) {
	t.Helper()
	var failure *fs.PathError
	if !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &failure) || failure.Path != path || failure.Op != op {
		t.Fatalf("capture error = %v, want ErrNotExist from %s on %s", err, op, path)
	}
}

// Embedding retains the real Name, Type and IsDir. Only Info is intercepted;
// neither this entry nor the walker filters any entry on the collector's behalf.
type recoveryObservedEntry struct {
	fs.DirEntry
	info func() (fs.FileInfo, error)
}

func (e recoveryObservedEntry) Info() (fs.FileInfo, error) { return e.info() }

func recoveryChangingWalk(t *testing.T, target string, cached bool, changed *int, change func() error) doctorTreeWalk {
	t.Helper()
	return func(root string, visit fs.WalkDirFunc) error {
		return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err == nil && path == target {
				if cached {
					// Fixture metadata is obtained before the change, independently of
					// calls made by the collector after this entry is delivered.
					info, infoErr := entry.Info()
					if infoErr != nil {
						t.Fatal("prepare cached entry:", infoErr)
					}
					entry = recoveryObservedEntry{DirEntry: entry, info: func() (fs.FileInfo, error) { return info, nil }}
				}
				if changeErr := change(); changeErr != nil {
					t.Fatal("prepare filesystem change:", changeErr)
				}
				*changed++
			}
			return visit(path, entry, err)
		})
	}
}
