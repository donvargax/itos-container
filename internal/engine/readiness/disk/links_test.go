package disk

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

func TestCheckoutIndependenceContract(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("unreadable directories require non-root")
	}
	porttest.CheckoutIndependenceContract(t, func() (port.Checkout, string, func(string)) {
		path := checkoutFixture(t)
		nested := filepath.Join(path, "nested")
		if err := os.Mkdir(nested, 0700); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(nested, "file")
		if err := os.WriteFile(file, []byte("read-only independent file"), 0444); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), "alias")
		c := &Checkout{}
		t.Cleanup(c.Close)
		t.Cleanup(func() { _ = os.Chmod(nested, 0700) })
		return c, path, func(change string) {
			switch change {
			case "shared":
				if err := os.Link(file, outside); err != nil {
					t.Fatal(err)
				}
			case "unavailable":
				if err := os.Chmod(nested, 0000); err != nil {
					t.Fatal(err)
				}
			case "independent":
				if err := os.Chmod(nested, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(outside); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			default:
				t.Fatal(change)
			}
		}
	})
}

func TestFileIndependenceNeedsNoContentReadOrSymlinkTraversal(t *testing.T) {
	path := checkoutFixture(t)
	unreadable := filepath.Join(path, "content")
	if err := os.WriteFile(unreadable, []byte("do not read"), 0000); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, outside+"-alias"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(path, "symlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(path, "directory-symlink")); err != nil {
		t.Fatal(err)
	}
	c := &Checkout{}
	if f := c.Pin(path); f != port.None {
		t.Fatal(f)
	}
	defer c.Close()
	if _, f := c.Inspect(); f != port.None || !c.Unchanged() {
		t.Fatal("read file contents or followed symlink", f)
	}
	if err := os.Link(unreadable, filepath.Join(t.TempDir(), "alias")); err != nil {
		t.Fatal(err)
	}
	if _, f := c.Inspect(); f != port.CheckoutHardlink || c.Unchanged() {
		t.Fatal("unreadable shared file admitted", f)
	}
}

func TestIndependenceWalkUsesPinnedTree(t *testing.T) {
	path := checkoutFixture(t)
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path + ".old") })
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(path+".old", ".git", "HEAD")
	if err := os.Link(original, filepath.Join(t.TempDir(), "alias")); err != nil {
		t.Fatal(err)
	}
	if f := independentFiles(root); f != port.CheckoutHardlink {
		t.Fatal("inspected replacement instead of pinned checkout", f)
	}
}

func TestIndependenceWalkRefusesMissingAndReplacedDirectories(t *testing.T) {
	path := checkoutFixture(t)
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := root.Lstat(".git")
	if err != nil {
		t.Fatal(err)
	}
	if f := independentDirectory(root, "missing", before); f != port.CheckoutUnavailable {
		t.Fatal(f)
	}
	if err := os.Rename(filepath.Join(path, ".git"), filepath.Join(path, ".git-old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(path, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if f := independentDirectory(root, ".git", before); f != port.CheckoutChanged {
		t.Fatal(f)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if f := independentFiles(root); f != port.CheckoutUnavailable {
		t.Fatal(f)
	}
}

type linkInfo struct {
	fs.FileInfo
	stat any
}

func (i linkInfo) Sys() any { return i.stat }

func TestLinkObservationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		stat any
		want port.Failure
	}{
		{"missing", nil, port.CheckoutUnavailable},
		{"not a structure", uint64(1), port.CheckoutUnavailable},
		{"no count", &struct{ Dev uint64 }{1}, port.CheckoutUnavailable},
		{"wrong count type", &struct{ Nlink string }{"1"}, port.CheckoutUnavailable},
		{"one", &struct{ Nlink uint64 }{1}, port.None},
		{"two", &struct{ Nlink uint64 }{2}, port.CheckoutHardlink},
		{"zero", &struct{ Nlink uint64 }{0}, port.CheckoutHardlink},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := independentFile(linkInfo{stat: tc.stat}); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}

func TestIndependenceUnavailableObservations(t *testing.T) {
	path := checkoutFixture(t)
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if f := inspectFiles(root, func(r *os.Root, name string) (*os.File, error) {
		f, err := r.Open(name)
		if err == nil {
			err = f.Close()
		}
		return f, err
	}); f != port.CheckoutUnavailable {
		t.Fatal("failed directory enumeration admitted", f)
	}
	// Enumerate a name which has disappeared from the pinned tree. The actual
	// Lstat fails; no synthetic metadata substitutes for the filesystem.
	vanished := t.TempDir()
	mustWrite(t, filepath.Join(vanished, "vanished-entry"))
	if f := inspectFiles(root, func(*os.Root, string) (*os.File, error) { return os.Open(vanished) }); f != port.CheckoutUnavailable {
		t.Fatal("missing file admitted", f)
	}
	before, err := root.Lstat(".git")
	if err != nil {
		t.Fatal(err)
	}
	if f := inspectDirectory(root, ".git", before, func(r *os.Root, name string) (*os.Root, error) {
		child, err := r.OpenRoot(name)
		if err == nil {
			err = child.Close()
		}
		return child, err
	}); f != port.CheckoutChanged {
		t.Fatal("unobservable directory identity admitted", f)
	}
}
