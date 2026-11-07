package disk

import (
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Workspace walks beneath a separately validated checkout without following
// symlinks. Keeping every parent pinned also detects replacement of an ancestor.
type Workspace struct {
	// MountStat is an adapter syscall seam; production uses os.Stat. It never
	// chooses the pathname, which must come from the freshly inspected init PID.
	MountStat func(string) (fs.FileInfo, error)
	source    *os.File
	binding   port.Binding
	roots     []*os.Root
	names     []string
	infos     []fs.FileInfo
	rootInfo  fs.FileInfo
	ops       *workspaceOperations
}

// Preserve real pinned handles while making failures between inspection and
// opening a child reproducible in adapter tests.
type workspaceOperations struct {
	openChild func(*os.Root, string) (*os.Root, error)
}

func (w *Workspace) Pin(b port.Binding) port.Outcome {
	w.Close()
	w.binding = b
	ops := workspaceOperations{openChild: (*os.Root).OpenRoot}
	if w.ops != nil {
		ops = *w.ops
	}
	if b.Workspace != "/workspace" && !strings.HasPrefix(b.Workspace, "/workspace/") {
		return port.Refused
	}
	root, err := os.OpenRoot(b.Checkout.Root)
	if err != nil {
		return port.Refused
	}
	w.roots = append(w.roots, root)
	w.rootInfo, err = root.Stat(".")
	if err != nil {
		w.Close()
		return port.Refused
	}
	for _, name := range strings.Split(strings.TrimPrefix(b.Workspace, "/workspace/"), "/") {
		if b.Workspace == "/workspace" {
			break
		}
		if name == "" || name == "." || name == ".." {
			w.Close()
			return port.Refused
		}
		info, err := root.Lstat(name)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			w.Close()
			return port.Refused
		}
		next, err := ops.openChild(root, name)
		if err != nil {
			w.Close()
			return port.Refused
		}
		w.roots = append(w.roots, next)
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			w.Close()
			return port.Refused
		}
		w.names, w.infos = append(w.names, name), append(w.infos, info)
		root = next
	}
	return ""
}
func (w *Workspace) Validate() port.Outcome {
	if len(w.roots) == 0 {
		return port.Refused
	}
	root, err := os.Lstat(w.binding.Checkout.Root)
	if err != nil || !os.SameFile(root, w.rootInfo) {
		return port.Refused
	}
	for i, name := range w.names {
		info, err := w.roots[i].Lstat(name)
		if err != nil || !os.SameFile(w.infos[i], info) {
			return port.Refused
		}
	}
	return ""
}
func (w *Workspace) Close() {
	if w.source != nil {
		_ = w.source.Close()
		w.source = nil
	}
	for _, root := range w.roots {
		_ = root.Close()
	}
	w.roots, w.names, w.infos = nil, nil, nil
}

func (w *Workspace) Source() (port.MountSource, port.Outcome) {
	if len(w.roots) == 0 {
		return port.MountSource{}, port.MountRefused
	}
	if w.source == nil {
		var err error
		w.source, err = openPath(w.roots[0])
		if err != nil {
			return port.MountSource{}, port.MountUnavailable
		}
	}
	info, err := w.source.Stat()
	if err != nil || !info.IsDir() {
		return port.MountSource{}, port.MountRefused
	}
	device, inode, ok := mountIdentity(info)
	if !ok {
		return port.MountSource{}, port.MountUnavailable
	}
	return port.MountSource{Path: fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), w.source.Fd()), Device: device, Inode: inode}, ""
}

func (w *Workspace) Mounted(pid int) (port.MountSource, port.Outcome) {
	if pid <= 0 {
		return port.MountSource{}, port.MountRefused
	}
	stat := w.MountStat
	if stat == nil {
		stat = os.Stat
	}
	info, err := stat(fmt.Sprintf("/proc/%d/root/workspace", pid))
	if err != nil || !info.IsDir() {
		return port.MountSource{}, port.MountRefused
	}
	device, inode, ok := mountIdentity(info)
	if !ok {
		return port.MountSource{}, port.MountRefused
	}
	return port.MountSource{Device: device, Inode: inode}, ""
}

func mountIdentity(info fs.FileInfo) (string, string, bool) {
	v := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return "", "", false
	}
	d, i := v.FieldByName("Dev"), v.FieldByName("Ino")
	if !d.IsValid() || !i.IsValid() || !d.CanUint() || !i.CanUint() {
		return "", "", false
	}
	return strconv.FormatUint(d.Uint(), 10), strconv.FormatUint(i.Uint(), 10), true
}
