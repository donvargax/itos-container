package disk

import (
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestCheckoutSocketScan(t *testing.T) {
	for _, name := range []string{"socket", "nested/a/b/c/d/e/socket", ".git/socket", ".git/nested/a/b/c/d/e/socket", strings.Repeat("deep/", 100) + "socket"} {
		t.Run(name, func(t *testing.T) {
			for _, mode := range []fs.FileMode{fs.ModeSocket, fs.ModeSocket | 0777} {
				tree := fstest.MapFS{name: {Mode: mode}, "regular": {Data: []byte("keep")}}
				if got := checkoutSockets(tree); got != port.IsolationUnverified {
					t.Fatalf("socket %s (%v): %s", name, mode, got)
				}
			}
		})
	}
	for _, tree := range []fstest.MapFS{
		{},
		{"regular": {Data: []byte("keep")}, ".git/nested/file": {Data: []byte("metadata")}},
		{"link": {Mode: fs.ModeSymlink, Data: []byte("../outside")}, "pipe": {Mode: fs.ModeNamedPipe}},
	} {
		if got := checkoutSockets(tree); got != "" {
			t.Fatalf("socket-free checkout: %s", got)
		}
	}
}

type unreadableSocketTree struct {
	fs.FS
	path string
}

func (f unreadableSocketTree) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.path {
		return nil, fs.ErrPermission
	}
	return fs.ReadDir(f.FS, name)
}

func TestCheckoutSocketScanRefusesIncompleteTraversal(t *testing.T) {
	tree := fstest.MapFS{"nested/file": {Data: []byte("keep")}}
	for _, name := range []string{".", "nested"} {
		if got := checkoutSockets(unreadableSocketTree{tree, name}); got != port.IsolationInspection {
			t.Fatalf("unreadable %s: %s", name, got)
		}
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = root.Close()
	if got := checkoutSockets(root.FS()); got != port.IsolationInspection {
		t.Fatalf("closed root: %s", got)
	}
}

func TestWorkspaceSocketContractWithFake(t *testing.T) {
	w := &porttest.Workspace{}
	porttest.WorkspaceSocketContract(t, w, port.Binding{}, func(present bool) {
		w.Failure = ""
		if present {
			w.Failure = port.IsolationUnverified
		}
	})
}

func TestWorkspaceSocketContractWithDisk(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native socket inventory targets Linux; portable scan and fake contracts run on every host")
	}
	for _, network := range []string{"unix", "unixpacket", "unixgram"} {
		for _, name := range []string{"socket", "nested/a/b/c/d/e/socket", ".git/socket", ".git/nested/a/b/c/d/e/socket"} {
			for _, stale := range []bool{false, true} {
				t.Run(network+"/"+name+"/stale="+strconv.FormatBool(stale), func(t *testing.T) {
					// Unix socket names have a small native length limit. Keep the
					// parent independent of the much longer test/subtest name.
					root, err := os.MkdirTemp("", "slice-19-")
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.RemoveAll(root) })
					path := filepath.Join(root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(filepath.Join(root, "workspace"), 0700); err != nil {
						t.Fatal(err)
					}
					keep := filepath.Join(root, "workspace", "keep")
					if err := os.WriteFile(keep, []byte("unchanged"), 0600); err != nil {
						t.Fatal(err)
					}
					b := port.Binding{Workspace: "/workspace/workspace"}
					b.Checkout.Root = root
					w := &Workspace{}
					var before fs.FileInfo
					porttest.WorkspaceSocketContract(t, w, b, func(present bool) {
						if present {
							closeSocket := bindCheckoutSocket(t, network, path)
							t.Cleanup(closeSocket)
							if stale {
								closeSocket()
							}
							before, err = os.Lstat(path)
							if err != nil || before.Mode()&os.ModeSocket == 0 {
								t.Fatalf("fixture is not a socket: %v", err)
							}
							return
						}
						after, err := os.Lstat(path)
						if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
							t.Fatalf("refusal changed the socket: %v", err)
						}
						data, err := os.ReadFile(keep)
						if err != nil || string(data) != "unchanged" {
							t.Fatalf("refusal changed checkout data: %v", err)
						}
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					})
				})
			}
		}
	}
}

func bindCheckoutSocket(t *testing.T, network, path string) func() {
	t.Helper()
	addr := &net.UnixAddr{Name: path, Net: network}
	if network == "unixgram" {
		conn, err := net.ListenUnixgram(network, addr)
		if err != nil {
			t.Fatal(err)
		}
		return func() { _ = conn.Close() }
	}
	listener, err := net.ListenUnix(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	return func() { _ = listener.Close() }
}

func TestWorkspaceSocketScanDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	for name, target := range map[string]string{"outside": outside, "loop": ".", "broken": "missing"} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	b := port.Binding{Workspace: "/workspace"}
	b.Checkout.Root = root
	w := &Workspace{}
	t.Cleanup(w.Close)
	if got := w.Pin(b); got != "" || w.Validate() != "" {
		t.Fatalf("symlink affected socket-free checkout: %s", got)
	}
}

func TestWorkspaceDoesNotScanSocketOutsideCheckout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native Unix socket fixture targets Linux")
	}
	root, err := os.MkdirTemp("", "slice-19-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	checkout := filepath.Join(root, "checkout")
	if err := os.Mkdir(checkout, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(checkout, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../socket", filepath.Join(checkout, "socket-link")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bindCheckoutSocket(t, "unix", filepath.Join(root, "socket")))
	b := port.Binding{Workspace: "/workspace"}
	b.Checkout.Root = checkout
	w := &Workspace{}
	t.Cleanup(w.Close)
	if got := w.Pin(b); got != "" || w.Validate() != "" {
		t.Fatalf("followed symlink to an outside socket: %s", got)
	}
}

func TestWorkspaceSocketScanFailureReleasesPin(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "workspace"), 0700); err != nil {
		t.Fatal(err)
	}
	b := port.Binding{Workspace: "/workspace/workspace"}
	b.Checkout.Root = root
	var handles []*os.Root
	w := &Workspace{ops: &workspaceOperations{openChild: func(parent *os.Root, name string) (*os.Root, error) {
		child, err := parent.OpenRoot(name)
		if err == nil {
			handles = append(handles, parent, child)
			_ = parent.Close() // The subsequent complete-checkout scan cannot finish.
		}
		return child, err
	}}}
	t.Cleanup(w.Close)
	if got := w.Pin(b); got != port.IsolationInspection || w.Validate() != port.Refused || len(w.roots) != 0 {
		t.Fatalf("unreadable scan retained workspace authority: %s", got)
	}
	for _, handle := range handles {
		if _, err := handle.Stat("."); err == nil {
			t.Fatal("unreadable scan retained an open handle")
		}
	}
	w.ops = nil
	if got := w.Pin(b); got != "" || w.Validate() != "" {
		t.Fatalf("retry after scan failure: %s", got)
	}
}
