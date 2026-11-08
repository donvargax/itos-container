package disk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestWorkspaceContracts(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		w := &porttest.Workspace{}
		porttest.WorkspaceContract(t, w, port.Binding{}, func() { w.Changed = true })
	})
	t.Run("disk", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "sub"), 0700); err != nil {
			t.Fatal(err)
		}
		b := port.Binding{Workspace: "/workspace/sub"}
		b.Checkout.Root = root
		w := &Workspace{}
		porttest.WorkspaceContract(t, w, b, func() {
			if err := w.roots[0].Rename("sub", "original"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "sub"), 0700); err != nil {
				t.Fatal(err)
			}
		})
	})
}

func TestWorkspacePinsAndRejectsSubstitution(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", "child"), 0700); err != nil {
		t.Fatal(err)
	}
	b := port.Binding{Workspace: "/workspace/sub/child"}
	b.Checkout.Root = root
	w := &Workspace{}
	t.Cleanup(w.Close)
	if f := w.Pin(b); f != "" || w.Validate() != "" {
		t.Fatal(f)
	}
	// Windows refuses renaming an ancestor with an open descendant, even with
	// delete sharing. Release only the leaf handle; both parents used to check
	// identity stay pinned. Prove this does not itself invalidate validation.
	if err := w.roots[len(w.roots)-1].Close(); err != nil {
		t.Fatal(err)
	}
	if w.Validate() != "" {
		t.Fatal("closing the leaf invalidated parent identity before substitution")
	}
	// The handle-relative rename permits the target directory itself to stay
	// open. Substituting its name must be detected by the still-pinned checkout.
	if err := w.roots[0].Rename("sub", "old"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub", "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if w.Validate() == "" {
		t.Fatal("replacement accepted")
	}
	w.Close()
	if w.Validate() == "" {
		t.Fatal("closed pin accepted")
	}
	b.Workspace = "/workspace"
	if f := w.Pin(b); f != "" || w.Validate() != "" {
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
		if w.Validate() == "" {
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
			if w.Validate() == "" || len(w.roots) != 0 || len(w.names) != 0 || len(w.infos) != 0 {
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
			if got := w.Pin(b); got != "" || w.Validate() != "" {
				t.Fatalf("fresh pin after failure = %s", got)
			}
		})
	}
}

func TestWorkspacePinClosesHandleOnRootStatFailure(t *testing.T) {
	b := port.Binding{Workspace: "/workspace"}
	b.Checkout.Root = t.TempDir()
	var opened *os.Root
	w := &Workspace{ops: &workspaceOperations{openRoot: func(path string) (*os.Root, error) {
		if path != b.Checkout.Root {
			t.Fatalf("opened different checkout: %s", path)
		}
		root, err := os.OpenRoot(path)
		if err != nil {
			t.Fatal(err)
		}
		opened = root
		// Fail the stat on the real descriptor after the root open succeeds.
		if err := root.Close(); err != nil {
			t.Fatal(err)
		}
		return root, nil
	}}}
	t.Cleanup(w.Close)
	if failure := w.Pin(b); failure != port.Refused {
		t.Fatal("unreadable root acquired authority", failure)
	}
	if opened == nil || len(w.roots) != 0 || w.Validate() != port.Refused {
		t.Fatal("failed root stat retained workspace authority")
	}
	if source, failure := w.Source(); failure != port.MountRefused || source != (port.MountSource{}) {
		t.Fatal("failed root stat supplied a mount source", source, failure)
	}
}
