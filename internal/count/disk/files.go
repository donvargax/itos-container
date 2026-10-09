// Package disk adapts read-only operating-system file access to the counting
// domain's Files port.
package disk

import (
	"errors"
	"io/fs"
	"os"

	"github.com/donvargax/itos-container/internal/count/port"
)

// Files reads input files from the local filesystem.
type Files struct{}

var _ port.Files = Files{}

// Read reads path without modifying or executing its contents and translates
// operating-system errors into the port's typed failures.
func (Files) Read(path string) ([]byte, port.ReadFailure) {
	data, err := os.ReadFile(path) //nolint:gosec // caller selects the input path through the Files port
	if err != nil {
		return nil, classifyReadError(path, err)
	}
	return data, nil
}

func classifyReadError(path string, err error) port.ReadFailure {
	if errors.Is(err, fs.ErrNotExist) {
		return &port.Missing{Path: path}
	}
	return &port.Unreadable{Path: path}
}
