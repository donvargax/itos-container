// Package disk reads selected checkout files without following symlinks.
package disk

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

// Files uses the effective checkout root supplied by command assembly. An empty
// Root means the working directory, matching the extension's existing seam.
type Files struct {
	Root string
	// OpenRoot duplicates an already pinned checkout root when supplied. The
	// caller retains its pin; every returned handle is owned by this reader.
	OpenRoot func() (*os.Root, error)
}

// Read checks every directory and opens the final regular file without blocking
// on a FIFO or following a link. No underlying error or source value is exposed.
func (f Files) Read(path string) ([]byte, port.Failure) {
	if !validPath(path) {
		return nil, port.Path
	}
	parts := strings.Split(path, "/")
	root, failure := f.directory(parts[:len(parts)-1])
	if failure != port.None {
		return nil, failure
	}
	defer func() { _ = root.Close() }()
	name := parts[len(parts)-1]
	info, err := root.Lstat(name)
	if err != nil {
		return nil, fileFailure(err)
	}
	if !info.Mode().IsRegular() {
		return nil, port.Path
	}
	return readCandidate(root, name, info)
}

// readCandidate checks the opened descriptor against the entry inspected before
// opening. Keeping this boundary explicit lets tests replace real filesystem
// entries between inspection and open without relying on a scheduler race.
func readCandidate(root *os.Root, name string, info fs.FileInfo) ([]byte, port.Failure) {
	file, err := openCandidate(root, name)
	if err != nil {
		return nil, fileFailure(err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, port.Path
	}
	return readBounded(file)
}

// Bound the opened descriptor's actual read, not a size that can change after Stat.
func readBounded(reader io.Reader) ([]byte, port.Failure) {
	data, err := io.ReadAll(io.LimitReader(reader, port.MaxBytes+1))
	if err != nil {
		return nil, port.Path
	}
	if len(data) > port.MaxBytes {
		return nil, port.Complexity
	}
	return data, port.None
}

// Directory requires an existing directory at every component below the root.
func (f Files) Directory(path string) bool {
	if !validPath(path) {
		return false
	}
	root, failure := f.directory(strings.Split(path, "/"))
	if failure != port.None {
		return false
	}
	_ = root.Close()
	return true
}

func validPath(path string) bool {
	return path != "." && fs.ValidPath(path) && !strings.ContainsAny(path, "\\\x00")
}

func (f Files) directory(parts []string) (*os.Root, port.Failure) {
	return f.walkDirectories(parts, (*os.Root).OpenRoot)
}

// walkDirectories takes the real directory opener separately so adapter tests
// can change an entry after inspection and before opening it, deterministically.
func (f Files) walkDirectories(parts []string, open func(*os.Root, string) (*os.Root, error)) (*os.Root, port.Failure) {
	path := f.Root
	if path == "" {
		path = "."
	}
	openRoot := f.OpenRoot
	if openRoot == nil {
		openRoot = func() (*os.Root, error) { return os.OpenRoot(path) }
	}
	root, err := openRoot()
	if err != nil {
		return nil, port.Path
	}
	for _, part := range parts {
		info, err := root.Lstat(part)
		if err != nil {
			_ = root.Close()
			return nil, fileFailure(err)
		}
		if !info.IsDir() {
			_ = root.Close()
			return nil, port.Path
		}
		next, err := open(root, part)
		if err != nil {
			// Windows can report a replaced non-directory as ErrNotExist.
			// Re-observe the entry while the parent is pinned: an existing
			// replacement is unsafe, not an absent requested directory.
			failure := fileFailure(err)
			if _, observed := root.Lstat(part); observed == nil {
				failure = port.Path
			}
			_ = root.Close()
			return nil, failure
		}
		_ = root.Close()
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			_ = next.Close()
			return nil, port.Path
		}
		root = next
	}
	return root, port.None
}

func fileFailure(err error) port.Failure {
	if errors.Is(err, fs.ErrNotExist) {
		return port.Missing
	}
	return port.Path
}
