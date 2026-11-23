package porttest

import (
	"context"

	sockets "github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
)

// Sockets returns a configured selection and records each call's checkout and
// whether its context had ended, without I/O.
type Sockets struct {
	Selection sockets.Selection
	Checkouts []string
	Ended     []bool
	// Deadline is the deadline of the last call's context, if it had one.
	Deadline bool
}

// Detect supplies the configured selection rather than observing a host.
func (s *Sockets) Detect(ctx context.Context, checkout string) sockets.Selection {
	return sockets.Selection{}
}
