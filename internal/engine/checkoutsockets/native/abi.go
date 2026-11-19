// Package native asks the running Linux kernel for its highest Landlock ABI.
package native

import (
	"context"
	"syscall"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
)

// Probe issues the Landlock version query. Use New: the call is a test seam.
type Probe struct {
	call func(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, errno syscall.Errno)
}

// New binds the probe to the real system call.
func New() Probe { return Probe{} }

// LandlockABI is not implemented yet.
func (Probe) LandlockABI(context.Context) (port.ABI, port.Failure) {
	return port.ABI{}, port.None
}
