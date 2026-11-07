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
	ops   *workspaceOperations
}

// Preserve real pinned handles while making failures between inspection and
// opening a child reproducible in adapter tests.
type workspaceOperations struct {
	openChild func(*os.Root, string) (*os.Root, error)
}

func (w *Workspace) Pin(b port.Binding) port.Outcome {
	w.Close()
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
	if failure := checkoutSockets(w.roots[0].FS()); failure != "" {
		w.Close()
		return failure
	}
	return ""
}
func (w *Workspace) Validate() port.Outcome {
	if len(w.roots) == 0 {
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
	for _, root := range w.roots {
		_ = root.Close()
	}
	w.roots, w.names, w.infos = nil, nil, nil
}
