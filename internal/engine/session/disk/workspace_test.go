package disk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestWorkspacePinsAndRejectsSubstitution(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", "child"), 0700); err != nil {
		t.Fatal(err)
	}
	b := port.Binding{Workspace: "/workspace/sub/child"}
	b.Checkout.Root = root
	w := &Workspace{}
	t.Cleanup(w.Close)
	if f := w.Pin(b); f != "" || !w.Unchanged() {
		t.Fatal(f)
	}
	// Use the pinned parent for the rename too. On Windows this uses the
	// handle-relative rename API instead of MoveFileEx on an open directory.
	if err := w.roots[0].Rename("sub", "old"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub", "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if w.Unchanged() {
		t.Fatal("replacement accepted")
	}
	w.Close()
	if w.Unchanged() {
		t.Fatal("closed pin accepted")
	}
	b.Workspace = "/workspace"
	if f := w.Pin(b); f != "" || !w.Unchanged() {
		t.Fatal(f)
	}
	w.Close()
}
func TestWorkspaceRefusesMissingAndAliasedPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/elsewhere", "/workspace/", "/workspace/.", "/workspace/..", "/workspace/missing", "/workspace/file", "/workspace/link"} {
		b := port.Binding{Workspace: path}
		b.Checkout.Root = root
		w := &Workspace{}
		if f := w.Pin(b); f != port.Refused {
			t.Fatalf("%s: %s", path, f)
		}
		if w.Unchanged() {
			t.Fatal("failure retained pin")
		}
	}
	b := port.Binding{Workspace: "/workspace"}
	b.Checkout.Root = filepath.Join(root, "missing")
	if f := (&Workspace{}).Pin(b); f != port.Refused {
		t.Fatal(f)
	}
}

func TestWorkspacePinClosesHandlesOnChildFailure(t *testing.T) {
	for _, fault := range []string{"open", "stat", "replacement"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "sub", "child"), 0700); err != nil {
				t.Fatal(err)
			}
			var handles []*os.Root
			ops := workspaceOperations{}
			ops.openChild = func(parent *os.Root, name string) (*os.Root, error) {
				handles = append(handles, parent)
				if name == "child" {
					switch fault {
					case "open":
						return nil, fs.ErrPermission
					case "replacement":
						if err := parent.Rename(name, "original"); err != nil {
							t.Fatal(err)
						}
						if err := parent.Mkdir(name, 0700); err != nil {
							t.Fatal(err)
						}
					}
				}
				child, err := parent.OpenRoot(name)
				if err == nil {
					handles = append(handles, child)
					if name == "child" && fault == "stat" {
						// The actual opened descriptor fails, without replacing the
						// earlier Lstat evidence or bypassing child traversal.
						_ = child.Close()
					}
				}
				return child, err
			}
			b := port.Binding{Workspace: "/workspace/sub/child"}
			b.Checkout.Root = root
			w := &Workspace{ops: &ops}
			t.Cleanup(w.Close)
			if got := w.Pin(b); got != port.Refused {
				t.Fatalf("%s during pin = %s, want refusal", fault, got)
			}
			if w.Unchanged() || len(w.roots) != 0 || len(w.names) != 0 || len(w.infos) != 0 {
				t.Fatal("failed pin retained workspace authority")
			}
			if len(handles) < 3 {
				t.Fatal("fault did not reach the nested child")
			}
			for _, handle := range handles {
				if _, err := handle.Stat("."); !errors.Is(err, os.ErrClosed) {
					t.Fatalf("failed pin retained an open handle: %v", err)
				}
			}
			w.ops = nil
			if got := w.Pin(b); got != "" || !w.Unchanged() {
				t.Fatalf("fresh pin after failure = %s", got)
			}
		})
	}
}
