// Package domain coordinates read-only requirement inspection.
package domain

import (
	"github.com/donvargax/itos-container/internal/doctor/port"
	policy "github.com/donvargax/itos-container/internal/policy/port"
)

// Inspect validates policy before later manifest, host or approval inspection.
// Only policy inspection is implemented in this slice.
func Inspect(policies port.Policies) policy.Result { return policies.Inspect() }
