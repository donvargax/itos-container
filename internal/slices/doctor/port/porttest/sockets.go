package porttest

import (
	"context"
	"time"

	sockets "github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Sockets returns a configured selection and records each call's checkout and
// whether its context had ended, without I/O.
type Sockets struct {
	Selection sockets.Selection
	Checkouts []string
	Engines   []readiness.EngineIdentity
	Ended     []bool
	// Deadline is the deadline of the last call's context, if it had one.
	Deadline time.Time
}

// Detect supplies the configured selection rather than observing a host.
func (s *Sockets) Detect(ctx context.Context, checkout string, engine readiness.EngineIdentity) sockets.Selection {
	s.Checkouts = append(s.Checkouts, checkout)
	s.Engines = append(s.Engines, engine)
	s.Ended = append(s.Ended, ctx.Err() != nil)
	s.Deadline, _ = ctx.Deadline()
	return s.Selection
}
