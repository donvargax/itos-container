// Package domain selects the checkout socket backend a host offers.
package domain

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
)

// MinimumLandlockABI is the first Landlock ABI with LANDLOCK_ACCESS_FS_RESOLVE_UNIX.
const MinimumLandlockABI = 9

// supported is the only platform whose kernel and daemon this detector reads.
var supported = port.Platform{OS: "linux", Architecture: "amd64"}

// Detector selects from observations made afresh on every call, so a change
// since an earlier call is visible to the next. The zero value probes nothing
// and certifies nothing.
type Detector struct {
	Kernel   port.Kernel
	Runtimes port.Runtimes
}

// Detect returns the backend the host offers, or a typed failure with no
// backend. Landlock at ABI 9 or later settles the answer without the runtime
// lookup. Only a completed lookup that confirms the absence of runsc, after the
// kernel confirmed no usable Landlock, yields Neither.
func (d Detector) Detect(ctx context.Context, platform port.Platform) port.Selection {
	if platform != supported {
		return failed(port.Unsupported)
	}
	if ctx.Err() != nil {
		return failed(port.Cancelled)
	}
	if d.Kernel == nil || d.Runtimes == nil {
		return failed(port.NoProbe)
	}
	abi, failure := d.Kernel.LandlockABI(ctx)
	if failure != port.None {
		return failed(failure)
	}
	switch {
	case abi.Absent && abi.Level == 0:
		// The kernel confirmed it has no usable Landlock: look for gVisor.
	case abi.Absent || abi.Level <= 0:
		return failed(port.ABIInvalid)
	case abi.Level >= MinimumLandlockABI:
		return port.Selection{Backend: port.Landlock}
	}
	registration, failure := d.Runtimes.Runsc(ctx)
	switch {
	case failure != port.None:
		return failed(failure)
	case registration == port.RunscRegistered:
		return port.Selection{Backend: port.GVisor}
	case registration == port.RunscAbsent:
		return port.Selection{Backend: port.Neither}
	}
	return failed(port.RuntimeInspect)
}

func failed(failure port.Failure) port.Selection { return port.Selection{Failure: failure} }
