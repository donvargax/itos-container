// Package disk reads only the policy in the effective working directory.
package disk

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

// Files supplies the fixed policy file; core configuration is unrelated.
type Files struct {
	// OpenRoot supplies a duplicate of a pinned checkout root, owned by Read.
	// Nil preserves the independent file-only mode's working-directory read.
	OpenRoot func() (*os.Root, error)
}

const maxPolicyBytes = 64 * 1024

// Read maps filesystem outcomes without returning source data in an error.
func (f Files) Read() ([]byte, port.Failure) {
	file, err := f.open()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, port.Missing
	}
	if err != nil {
		return nil, port.Unreadable
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxPolicyBytes {
		return nil, port.Unreadable
	}
	return readBounded(file)
}

func (f Files) open() (*os.File, error) {
	if f.OpenRoot == nil {
		return openPolicy()
	}
	root, err := f.OpenRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.OpenFile("itos-container.yaml", policyOpenFlags, 0)
}

// Bound the actual read as well as the initial size: the file can grow after Stat.
func readBounded(reader io.Reader) ([]byte, port.Failure) {
	data, err := io.ReadAll(io.LimitReader(reader, maxPolicyBytes+1))
	if err != nil || len(data) > maxPolicyBytes {
		return nil, port.Unreadable
	}
	return data, port.None
}
