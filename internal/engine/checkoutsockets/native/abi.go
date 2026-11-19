// Package native asks the running Linux kernel for its highest Landlock ABI.
package native

import (
	"context"
	"syscall"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
)

// Probe issues landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION).
// The call is a seam for tests; New binds the real system call, and the zero
// value does as well.
type Probe struct {
	call func(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, errno syscall.Errno)
}

// New binds the probe to the real system call.
func New() Probe { return Probe{call: rawSyscall} }

// LandlockABI asks the kernel afresh. ENOSYS (unimplemented) and EOPNOTSUPP
// (disabled) confirm that Landlock is absent; any other error, or a successful
// reply with a nonpositive ABI, is a failure with no answer.
func (p Probe) LandlockABI(ctx context.Context) (port.ABI, port.Failure) {
	if ctx.Err() != nil {
		return port.ABI{}, port.Cancelled
	}
	call := p.call
	if call == nil {
		call = rawSyscall
	}
	reply, _, errno := call(versionCall, 0, 0, versionFlag)
	switch {
	case errno == syscall.ENOSYS || errno == syscall.EOPNOTSUPP:
		return port.ABI{Absent: true}, port.None
	case errno != 0:
		return port.ABI{}, port.ABIQuery
	}
	level := int(reply)
	if level <= 0 {
		return port.ABI{}, port.ABIInvalid
	}
	return port.ABI{Level: level}, port.None
}
