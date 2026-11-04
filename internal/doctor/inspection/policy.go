// Package inspection connects doctor to the reusable policy engine.
package inspection

import (
	"github.com/donvargax/itos-container/internal/policy"
	"github.com/donvargax/itos-container/internal/policy/port"
)

// Policies supplies strict policy inspection through a data source.
type Policies struct{ Source port.Source }

// Inspect returns a complete validated policy or its refusals.
func (p Policies) Inspect() port.Result { return policy.Inspect(p.Source) }
