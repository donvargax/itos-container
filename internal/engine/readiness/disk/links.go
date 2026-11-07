package disk

import (
	"io"
	"io/fs"
	"os"
	"reflect"
	"time"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

const (
	checkoutBatchSize = 128
	checkoutEntries   = 100000
	checkoutDepth     = 64
	checkoutScanTime  = 10 * time.Second
)

// One budget spans the entire traversal, never one allowance per directory.
// Time is checked between synchronous filesystem calls; no abandoned scanner
// continues consuming resources after refusal. Kernel calls are not preempted.
type fileScan struct {
	remaining int
	deadline  time.Time
	now       func() time.Time
	readDir   func(*os.File, int) ([]os.DirEntry, error)
}

func newFileScan() *fileScan {
	return &fileScan{remaining: checkoutEntries, deadline: time.Now().Add(checkoutScanTime), now: time.Now, readDir: (*os.File).ReadDir}
}

func (s *fileScan) expired() bool { return !s.now().Before(s.deadline) }

// Inspect only metadata for files; read permissions and Git's ignore/tracking
// rules say nothing about shared writable inodes. Directory pins prevent a
// replaced intermediate pathname from redirecting traversal. Symlinks are not
// followed; existing root, Git and selected-file symlink rules still apply.
func independentFiles(root *os.Root) port.Failure {
	return newFileScan().inspectFiles(root, 0, (*os.Root).Open)
}

// Keep handle acquisition separate so adapter tests can exercise disappearing
// entries and failed handles deterministically, as the checkout pin tests do.
func (s *fileScan) inspectFiles(root *os.Root, depth int, open func(*os.Root, string) (*os.File, error)) port.Failure {
	if s.expired() {
		return port.CheckoutUnavailable
	}
	directory, err := open(root, ".")
	if err != nil {
		return port.CheckoutUnavailable
	}
	defer func() { _ = directory.Close() }()
	for {
		if s.expired() {
			return port.CheckoutUnavailable
		}
		entries, err := s.readDir(directory, checkoutBatchSize)
		if err != nil && err != io.EOF {
			return port.CheckoutUnavailable
		}
		for _, entry := range entries {
			if s.remaining == 0 || s.expired() {
				return port.CheckoutUnavailable
			}
			s.remaining--
			info, err := root.Lstat(entry.Name())
			if err != nil {
				return port.CheckoutUnavailable
			}
			if info.Mode().IsRegular() {
				if failure := independentFile(info); failure != port.None {
					return failure
				}
			} else if info.IsDir() {
				if failure := s.inspectDirectory(root, entry.Name(), info, depth+1, (*os.Root).OpenRoot); failure != port.None {
					return failure
				}
			}
		}
		if s.expired() {
			return port.CheckoutUnavailable
		}
		if err == io.EOF {
			return port.None
		}
	}
}

func (s *fileScan) inspectDirectory(parent *os.Root, name string, before fs.FileInfo, depth int, open func(*os.Root, string) (*os.Root, error)) port.Failure {
	if depth > checkoutDepth || s.expired() {
		return port.CheckoutUnavailable
	}
	child, err := open(parent, name)
	if err != nil {
		return port.CheckoutUnavailable
	}
	defer func() { _ = child.Close() }()
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		return port.CheckoutChanged
	}
	return s.inspectFiles(child, depth, (*os.Root).Open)
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
