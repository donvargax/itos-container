package disk

import (
	"io/fs"
	"os"
	"reflect"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Inspect only metadata for files; read permissions and Git's ignore/tracking
// rules say nothing about shared writable inodes. Directory pins prevent a
// replaced intermediate pathname from redirecting traversal. Symlinks are not
// followed; existing root, Git and selected-file symlink rules still apply.
func independentFiles(root *os.Root) port.Failure {
	return inspectFiles(root, (*os.Root).Open)
}

// Keep handle acquisition separate so adapter tests can exercise disappearing
// entries and failed handles deterministically, as the checkout pin tests do.
func inspectFiles(root *os.Root, open func(*os.Root, string) (*os.File, error)) port.Failure {
	directory, err := open(root, ".")
	if err != nil {
		return port.CheckoutUnavailable
	}
	defer func() { _ = directory.Close() }()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return port.CheckoutUnavailable
	}
	for _, entry := range entries {
		info, err := root.Lstat(entry.Name())
		if err != nil {
			return port.CheckoutUnavailable
		}
		if info.Mode().IsRegular() {
			if failure := independentFile(info); failure != port.None {
				return failure
			}
		} else if info.IsDir() {
			if failure := independentDirectory(root, entry.Name(), info); failure != port.None {
				return failure
			}
		}
	}
	return port.None
}

func independentDirectory(parent *os.Root, name string, before fs.FileInfo) port.Failure {
	return inspectDirectory(parent, name, before, (*os.Root).OpenRoot)
}

func inspectDirectory(parent *os.Root, name string, before fs.FileInfo, open func(*os.Root, string) (*os.Root, error)) port.Failure {
	child, err := open(parent, name)
	if err != nil {
		return port.CheckoutUnavailable
	}
	defer func() { _ = child.Close() }()
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		return port.CheckoutChanged
	}
	return independentFiles(child)
}

// Like checkout identity, native link metadata is observed without OS logic
// branches or build tags. Unsupported/missing observations are never safe.
func independentFile(info fs.FileInfo) port.Failure {
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct {
		return port.CheckoutUnavailable
	}
	links := stat.FieldByName("Nlink")
	if !links.IsValid() || !links.CanUint() {
		return port.CheckoutUnavailable
	}
	if links.Uint() != 1 {
		return port.CheckoutHardlink
	}
	return port.None
}
