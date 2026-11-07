package inspection

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/readiness/domain"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Readiness supplies host selection values to the reusable inspection engine.
type Readiness struct {
	Inspector domain.Inspector
	Input     port.Input
}

// Inspect returns observations without approval or persisted state.
func (r Readiness) Inspect() port.Result { return r.Inspector.Inspect(r.Input) }

// InspectBeforeEngine exposes the shared pinned local-checkpoint capability.
func (r Readiness) InspectBeforeEngine(check func(port.Result) bool) port.Result {
	return r.Inspector.InspectBeforeEngine(r.Input, check)
}

// InspectBeforeEngineContext supplies run's invocation scope to the shared engine.
func (r Readiness) InspectBeforeEngineContext(ctx context.Context, check func(port.Result) bool) port.Result {
	return r.Inspector.InspectBeforeEngineContext(ctx, r.Input, check)
}
