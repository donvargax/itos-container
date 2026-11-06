package disk

import (
	"io/fs"
	"os"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Workspace walks beneath a separately validated checkout without following
// symlinks. Keeping every parent pinned also detects replacement of an ancestor.
type Workspace struct {
	roots []*os.Root
	names []string
	infos []fs.FileInfo
}

func (w *Workspace) Pin(b port.Binding) port.Outcome {
	w.Close()
	if b.Workspace != "/workspace" && !strings.HasPrefix(b.Workspace, "/workspace/") {
		return port.Refused
	}
	root, err := os.OpenRoot(b.Checkout.Root)
	if err != nil {
		return port.Refused
	}
	w.roots = append(w.roots, root)
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
		next, err := root.OpenRoot(name)
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
func (w *Workspace) Unchanged() bool {
	if len(w.roots) == 0 {
		return false
	}
	for i, name := range w.names {
		info, err := w.roots[i].Lstat(name)
		if err != nil || !os.SameFile(w.infos[i], info) {
			return false
		}
	}
	return true
}
func (w *Workspace) Close() {
	for _, root := range w.roots {
		_ = root.Close()
	}
	w.roots, w.names, w.infos = nil, nil, nil
}
