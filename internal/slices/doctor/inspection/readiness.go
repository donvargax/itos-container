package inspection

import (
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
