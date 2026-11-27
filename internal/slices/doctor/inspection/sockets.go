package inspection

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/domain"
	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Sockets supplies the host's kernel probe, the runtime lookup for a validated
// checkout and the platform to the shared detector. Nothing is kept between
// calls, so each detection observes afresh.
type Sockets struct {
	Platform port.Platform
	Kernel   port.Kernel
	Runtimes func(checkout string, engine readiness.EngineIdentity) port.Runtimes
}

// Detect selects the backend the host offers. A missing lookup is a detector
// without its observations, which confirms nothing.
func (s Sockets) Detect(ctx context.Context, checkout string, engine readiness.EngineIdentity) port.Selection {
	detector := domain.Detector{Kernel: s.Kernel}
	if s.Runtimes != nil {
		detector.Runtimes = s.Runtimes(checkout, engine)
	}
	return detector.Detect(ctx, s.Platform)
}
