// Package domain coordinates read-only requirement inspection.
package domain

import (
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// Inspect validates policy before later manifest, host or approval inspection.
// Only policy inspection is implemented in this slice.
func Inspect(policies port.Policies) policy.Result { return policies.Inspect() }
