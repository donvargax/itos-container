// Package disk reads only the policy in the effective working directory.
package disk

import (
	"errors"
	"io/fs"
	"os"

	"github.com/donvargax/itos-container/internal/policy/port"
)

// Files supplies the fixed policy file; core configuration is unrelated.
type Files struct{}

// Read maps filesystem outcomes without returning source data in an error.
func (Files) Read() ([]byte, port.Failure) {
	data, err := os.ReadFile("itos-container.yaml")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, port.Missing
	}
	if err != nil {
		return nil, port.Unreadable
	}
	return data, port.None
}
