package disk

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

func TestPinAndInspectHandleFailures(t *testing.T) {
	path := checkoutFixture(t)
	c := &Checkout{}
	if _, failure := c.Inspect(); failure != port.CheckoutUnavailable {
		t.Fatal(failure)
	}
	failure := c.pin(path, func(path string) (*os.Root, port.Failure) {
		r, err := os.OpenRoot(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		return r, port.None
	})
	if failure != port.CheckoutUnavailable || c.root != nil {
		t.Fatal(failure, c.root)
	}
	if failure := c.Pin(path); failure != port.None {
		t.Fatal(failure)
	}
	defer c.Close()
	for range 2 {
		if _, failure := c.Inspect(); failure != port.None {
			t.Fatal(failure)
		}
	}
	if err := os.RemoveAll(filepath.Join(path, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, failure := c.Inspect(); failure != port.CheckoutLayout {
		t.Fatal(failure)
	}
}

func TestGitReplacementBetweenStatAndOpen(t *testing.T) {
	for _, change := range []string{"removed", "replaced", "closed"} {
		t.Run(change, func(t *testing.T) {
			path := checkoutFixture(t)
			c := &Checkout{}
			if failure := c.Pin(path); failure != port.None {
				t.Fatal(failure)
			}
			defer c.Close()
			_, failure := c.inspect(func(parent *os.Root, name string) (*os.Root, error) {
				if change != "closed" {
					if err := os.Rename(filepath.Join(path, name), filepath.Join(path, name+".old")); err != nil {
						t.Fatal(err)
					}
				}
				if change == "replaced" {
					if err := os.Mkdir(filepath.Join(path, name), 0700); err != nil {
						t.Fatal(err)
					}
				}
				root, err := parent.OpenRoot(name)
				if change == "closed" && err == nil {
					err = root.Close()
				}
				return root, err
			})
			want := port.CheckoutChanged
			if change == "removed" {
				want = port.CheckoutPath
			}
			if failure != want {
				t.Fatal(failure, want)
			}
		})
	}
}

func TestRootWalkChangesAndUnavailableBase(t *testing.T) {
	path := checkoutFixture(t)
	if root, failure := walkRoot(path, func(string) (*os.Root, error) { return nil, os.ErrPermission }, (*os.Root).OpenRoot); root != nil || failure != port.CheckoutUnavailable {
		t.Fatal(root, failure)
	}
	root, failure := openRoot("/")
	if failure != port.None {
		t.Fatal(failure)
	}
	_ = root.Close()
	for _, change := range []string{"removed", "replaced", "closed"} {
		t.Run(change, func(t *testing.T) {
			parent := checkoutFixture(t)
			target := filepath.Join(parent, "candidate")
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			root, failure := walkRoot(target, os.OpenRoot, func(root *os.Root, name string) (*os.Root, error) {
				if name != "candidate" {
					return root.OpenRoot(name)
				}
				if change != "closed" {
					if err := os.Rename(target, target+".old"); err != nil {
						t.Fatal(err)
					}
				}
				if change == "replaced" {
					if err := os.Mkdir(target, 0700); err != nil {
						t.Fatal(err)
					}
				}
				next, err := root.OpenRoot(name)
				if change == "closed" && err == nil {
					err = next.Close()
				}
				return next, err
			})
			want := port.CheckoutChanged
			if change == "removed" {
				want = port.CheckoutUnavailable
			}
			if root != nil || failure != want {
				t.Fatal(root, failure, want)
			}
		})
	}
}

func TestRootRemovalIsNotStable(t *testing.T) {
	path := checkoutFixture(t)
	c := &Checkout{}
	if failure := c.Pin(path); failure != port.None {
		t.Fatal(failure)
	}
	defer c.Close()
	if _, failure := c.Inspect(); failure != port.None {
		t.Fatal(failure)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path + ".old") })
	if c.Unchanged() {
		t.Fatal("missing root is stable")
	}
	c.Close()
	if _, err := c.OpenRoot(); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
}

func checkoutFixture(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("checkout identity adapter is used only on Linux")
	}
	path := t.TempDir()
	for _, name := range []string{".git/objects/info", ".git/refs"} {
		if err := os.MkdirAll(filepath.Join(path, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{".git/HEAD", ".git/config"} {
		if err := os.WriteFile(filepath.Join(path, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestCheckoutContract(t *testing.T) {
	for _, replaceGit := range []bool{false, true} {
		porttest.CheckoutContract(t, func() (port.Checkout, port.CheckoutIdentity, func()) {
			path := checkoutFixture(t)
			root, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			git, err := os.Stat(filepath.Join(path, ".git"))
			if err != nil {
				t.Fatal(err)
			}
			device, inode := identity(root)
			gd, gi := identity(git)
			c := &Checkout{}
			t.Cleanup(c.Close)
			return c, port.CheckoutIdentity{Root: path, Device: device, Inode: inode, GitDevice: gd, GitInode: gi}, func() {
				target := path
				if replaceGit {
					target = filepath.Join(path, ".git")
				}
				if err := os.Rename(target, target+".old"); err != nil {
					t.Fatal(err)
				}
				if !replaceGit {
					t.Cleanup(func() { _ = os.RemoveAll(target + ".old") })
				}
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestReadersUsePinnedRoot(t *testing.T) {
	path := checkoutFixture(t)
	c := &Checkout{}
	if _, err := c.OpenRoot(); err == nil {
		t.Fatal("opened before pin")
	}
	if failure := c.Pin(path); failure != port.None {
		t.Fatal(failure)
	}
	defer c.Close()
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path + ".old") })
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := c.OpenRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if data, err := root.ReadFile(".git/HEAD"); err != nil || string(data) != "fixture" {
		t.Fatal(string(data), err)
	}
	if c.Unchanged() {
		t.Fatal("replacement ignored")
	}
}

func TestCheckoutRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		want port.Failure
	}{
		{"relative", port.CheckoutRoot}, {"absent", port.CheckoutUnavailable}, {"file root", port.CheckoutUnavailable},
		{"gitfile", port.CheckoutLayout}, {"missing head", port.CheckoutLayout}, {"head directory", port.CheckoutLayout},
		{"root symlink", port.CheckoutPath}, {"git symlink", port.CheckoutPath}, {"head symlink", port.CheckoutPath},
		{"commondir", port.CheckoutLayout}, {"alternates", port.CheckoutLayout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := checkoutFixture(t)
			switch tc.name {
			case "relative":
				path = "relative"
			case "absent":
				path = filepath.Join(path, "absent")
			case "file root":
				path = filepath.Join(path, ".git/HEAD")
			case "gitfile":
				mustRemove(t, filepath.Join(path, ".git"))
				mustWrite(t, filepath.Join(path, ".git"))
			case "missing head":
				mustRemove(t, filepath.Join(path, ".git/HEAD"))
			case "head directory":
				mustRemove(t, filepath.Join(path, ".git/HEAD"))
				if err := os.Mkdir(filepath.Join(path, ".git/HEAD"), 0700); err != nil {
					t.Fatal(err)
				}
			case "root symlink":
				alias := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(path, alias); err != nil {
					t.Fatal(err)
				}
				path = alias
			case "git symlink", "head symlink":
				name := ".git"
				if tc.name == "head symlink" {
					name = ".git/HEAD"
				}
				target := filepath.Join(path, name)
				mustRemove(t, target)
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
			case "commondir":
				mustWrite(t, filepath.Join(path, ".git/commondir"))
			case "alternates":
				mustWrite(t, filepath.Join(path, ".git/objects/info/alternates"))
			}
			c := &Checkout{}
			defer c.Close()
			failure := c.Pin(path)
			var got port.CheckoutIdentity
			if failure == port.None {
				got, failure = c.Inspect()
			}
			if failure != tc.want || got != (port.CheckoutIdentity{}) {
				t.Fatal(got, failure)
			}
			c.Close()
			if _, err := c.OpenRoot(); err == nil {
				t.Fatal("failed Open retained a pin")
			}
		})
	}
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}
