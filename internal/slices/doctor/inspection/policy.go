// Package inspection connects doctor to the reusable policy engine.
package inspection

import (
	policy "github.com/donvargax/itos-container/internal/engine/policy/domain"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

// Policies supplies strict policy inspection through a data source.
type Policies struct{ Source port.Source }

// Inspect returns a complete validated policy or its refusals.
func (p Policies) Inspect() port.Result { return policy.Inspect(p.Source) }
