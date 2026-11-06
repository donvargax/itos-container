package disk

import (
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
	if f := w.Pin(b); f != "" || !w.Unchanged() {
		t.Fatal(f)
	}
	if err := os.Rename(filepath.Join(root, "sub"), filepath.Join(root, "old")); err != nil {
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
