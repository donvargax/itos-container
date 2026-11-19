// Package porttest supplies memory-only checkout socket fakes and the shared
// observation contracts their concrete adapters must also meet.
package porttest

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
)

// Kernel answers from memory. Calls counts every invocation, including refused
// ones; Reads counts only the observations it actually made.
type Kernel struct {
	ABI          port.ABI
	Failure      port.Failure
	Calls, Reads int
}

func (k *Kernel) LandlockABI(ctx context.Context) (port.ABI, port.Failure) {
	k.Calls++
	if ctx.Err() != nil {
		return port.ABI{}, port.Cancelled
	}
	k.Reads++
	if k.Failure != port.None {
		return port.ABI{}, k.Failure
	}
	return k.ABI, port.None
}

// Subject exposes the fake to KernelContract.
func (k *Kernel) Subject() KernelSubject {
	return KernelSubject{Kernel: k, Reads: func() int { return k.Reads }, Reply: func(r KernelReply) {
		k.ABI, k.Failure = port.ABI{}, port.None
		switch r.Answer {
		case ReplyLevel:
			k.ABI = port.ABI{Level: r.Level}
		case ReplyNotImplemented, ReplyDisabled:
			k.ABI = port.ABI{Absent: true}
		case ReplyDenied:
			k.Failure = port.ABIQuery
		case ReplyNonPositive:
			k.Failure = port.ABIInvalid
		}
	}}
}

// Runtimes answers from memory with the same counters as Kernel.
type Runtimes struct {
	Registration port.Registration
	Failure      port.Failure
	Calls, Reads int
}

func (r *Runtimes) Runsc(ctx context.Context) (port.Registration, port.Failure) {
	r.Calls++
	if ctx.Err() != nil {
		return port.RegistrationUnknown, port.Cancelled
	}
	r.Reads++
	if r.Failure != port.None {
		return port.RegistrationUnknown, r.Failure
	}
	return r.Registration, port.None
}

// Subject exposes the fake to RuntimesContract.
func (r *Runtimes) Subject() RuntimesSubject {
	return RuntimesSubject{Runtimes: r, Reads: func() int { return r.Reads }, Reply: func(reply RuntimesReply) {
		r.Registration, r.Failure = port.RegistrationUnknown, port.None
		switch reply {
		case ReplyRegistered:
			r.Registration = port.RunscRegistered
		case ReplyAbsent:
			r.Registration = port.RunscAbsent
		case ReplyUnrecognized:
			r.Failure = port.RuntimeRegistration
		case ReplyUnreachable:
			r.Failure = port.RuntimeInspect
		}
	}}
}
