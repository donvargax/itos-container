// Package disk pins read-only checkout identities without invoking Git.
package disk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Checkout retains handles until the snapshot has been checked for substitution.
type Checkout struct {
	root, git         *os.Root
	path              string
	rootInfo, gitInfo fs.FileInfo
}

// Pin validates and pins the selected root before reading policy or manifest.
// Git layout and identity are inspected only after the file prerequisites pass.
func (c *Checkout) Pin(path string) port.Failure {
	return c.pin(path, openRoot)
}

// Separate real open operations let adapter tests substitute entries between
// inspection and open, without relying on scheduler races or simulated files.
func (c *Checkout) pin(path string, open func(string) (*os.Root, port.Failure)) port.Failure {
	c.Close()
	if !filepath.IsAbs(path) {
		return port.CheckoutRoot
	}
	path = filepath.Clean(path)
	root, failure := open(path)
	if failure != port.None {
		return failure
	}
	c.root, c.path = root, path
	var err error
	c.rootInfo, err = root.Stat(".")
	if err != nil {
		c.Close()
		return port.CheckoutUnavailable
	}
	return port.None
}

// Inspect checks embedded Git storage without parsing configuration or following
// indirection. Its identity belongs to the root already pinned by Pin.
func (c *Checkout) Inspect() (port.CheckoutIdentity, port.Failure) {
	return c.inspect((*os.Root).OpenRoot)
}

func (c *Checkout) inspect(open func(*os.Root, string) (*os.Root, error)) (port.CheckoutIdentity, port.Failure) {
	if c.root == nil {
		return port.CheckoutIdentity{}, port.CheckoutUnavailable
	}
	if c.git != nil {
		_ = c.git.Close()
		c.git = nil
	}
	if failure := c.openGit(open); failure != port.None {
		return port.CheckoutIdentity{}, failure
	}
	device, inode := identity(c.rootInfo)
	gitDevice, gitInode := identity(c.gitInfo)
	return port.CheckoutIdentity{Root: c.path, Device: device, Inode: inode, GitDevice: gitDevice, GitInode: gitInode}, port.None
}

// OpenRoot duplicates the pinned root for policy and manifest readers. It never
// reopens the selected pathname, which may already name a replacement tree.
func (c *Checkout) OpenRoot() (*os.Root, error) {
	if c.root == nil {
		return nil, os.ErrClosed
	}
	return c.root.OpenRoot(".")
}

// Unchanged compares fresh path observations with both pinned directories.
func (c *Checkout) Unchanged() bool {
	if c.root == nil || c.git == nil {
		return false
	}
	current, failure := openRoot(c.path)
	if failure != port.None {
		return false
	}
	defer func() { _ = current.Close() }()
	root, rootErr := current.Stat(".")
	git, gitErr := current.Lstat(".git")
	return rootErr == nil && gitErr == nil && os.SameFile(c.rootInfo, root) && os.SameFile(c.gitInfo, git)
}

// Close releases all pins; it is safe after a failed Open or an earlier Close.
func (c *Checkout) Close() {
	if c.git != nil {
		_ = c.git.Close()
		c.git = nil
	}
	if c.root != nil {
		_ = c.root.Close()
		c.root = nil
	}
}

func (c *Checkout) openGit(open func(*os.Root, string) (*os.Root, error)) port.Failure {
	info, err := c.root.Lstat(".git")
	if err != nil {
		return port.CheckoutLayout
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return port.CheckoutPath
	}
	if !info.IsDir() {
		return port.CheckoutLayout
	}
	git, err := open(c.root, ".git")
	if err != nil {
		return port.CheckoutPath
	}
	c.git = git
	c.gitInfo, err = git.Stat(".")
	if err != nil || !os.SameFile(info, c.gitInfo) {
		return port.CheckoutChanged
	}
	for _, name := range []string{"HEAD", "config", "objects", "refs"} {
		entry, err := git.Lstat(name)
		if err != nil {
			return port.CheckoutLayout
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return port.CheckoutPath
		}
		directory := name == "objects" || name == "refs"
		if (directory && !entry.IsDir()) || (!directory && !entry.Mode().IsRegular()) {
			return port.CheckoutLayout
		}
	}
	for _, name := range []string{"commondir", "objects/info/alternates", "objects/info/http-alternates"} {
		if _, err := git.Lstat(name); !errors.Is(err, fs.ErrNotExist) {
			return port.CheckoutLayout
		}
	}
	return port.None
}

// Walk from the filesystem root using pinned parents. Lstat rejects aliases,
// and descriptor identity catches a replacement between Lstat and OpenRoot.
func openRoot(path string) (*os.Root, port.Failure) {
	return walkRoot(path, os.OpenRoot, (*os.Root).OpenRoot)
}

func walkRoot(path string, base func(string) (*os.Root, error), open func(*os.Root, string) (*os.Root, error)) (*os.Root, port.Failure) {
	volume := filepath.VolumeName(path)
	root, err := base(volume + string(filepath.Separator))
	if err != nil {
		return nil, port.CheckoutUnavailable
	}
	for _, name := range strings.Split(strings.TrimPrefix(path[len(volume):], string(filepath.Separator)), string(filepath.Separator)) {
		if name == "" {
			continue
		}
		info, err := root.Lstat(name)
		if err != nil {
			_ = root.Close()
			return nil, port.CheckoutUnavailable
		}
		if info.Mode()&os.ModeSymlink != 0 {
			_ = root.Close()
			return nil, port.CheckoutPath
		}
		if !info.IsDir() {
			_ = root.Close()
			return nil, port.CheckoutUnavailable
		}
		next, err := open(root, name)
		_ = root.Close()
		if err != nil {
			return nil, port.CheckoutUnavailable
		}
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			_ = next.Close()
			return nil, port.CheckoutChanged
		}
		root = next
	}
	return root, port.None
}

// Host readiness is Linux-only. Reflection keeps this adapter buildable on
// platforms that refuse readiness before opening it, without OS logic branches.
func identity(info fs.FileInfo) (string, string) {
	stat := reflect.ValueOf(info.Sys()).Elem()
	return strconv.FormatUint(stat.FieldByName("Dev").Uint(), 10), strconv.FormatUint(stat.FieldByName("Ino").Uint(), 10)
}
