package porttest

import (
	"context"
	"errors"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

// Fake is a stateful, I/O-free terminal for tests of terminal consumers.
type Fake struct {
	Size    port.Dimensions
	Invalid port.FailureKind
	State   int
}

func (f *Fake) Acquire(context.Context) (port.Lease, error) {
	return nil, errors.New("terminal acquisition not implemented")
}

func (f *Fake) Raw() bool                   { return false }
func (f *Fake) Snapshot() any               { return f.State }
func (f *Fake) ChangeState()                { f.State++ }
func (f *Fake) Resize(size port.Dimensions) { f.Size = size }
func (f *Fake) Hangup(_ bool)               {}
func (f *Fake) Break()                      {}
