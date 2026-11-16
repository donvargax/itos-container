// Package native implements the host-terminal port for an explicitly bound file.
package native

import (
	"context"
	"errors"
	"os"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

// Provider never discovers or closes a caller's terminal.
type Provider struct{}

// New binds a file and an explicit operating-system value.
func New(_ *os.File, _ string) *Provider { return &Provider{} }

// Acquire is a compiling red-first stub for T-56's shared contract.
func (*Provider) Acquire(context.Context) (port.Lease, error) {
	return nil, errors.New("terminal acquisition not implemented")
}
