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
type Files struct{ Root string }

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
	path := f.Root
	if path == "" {
		path = "."
	}
	root, err := os.OpenRoot(path)
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
		next, err := root.OpenRoot(part)
		_ = root.Close()
		if err != nil {
			return nil, fileFailure(err)
		}
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
