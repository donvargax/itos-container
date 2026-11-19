// Package domain selects the checkout socket backend a host offers.
package domain

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
)

// MinimumLandlockABI is the first Landlock ABI with LANDLOCK_ACCESS_FS_RESOLVE_UNIX.
const MinimumLandlockABI = 9

// Detector selects from observations made afresh on every call.
type Detector struct {
	Kernel   port.Kernel
	Runtimes port.Runtimes
}

// Detect returns the backend the host offers, or a typed failure with no backend.
func (Detector) Detect(context.Context, port.Platform) port.Selection {
	return port.Selection{}
}
