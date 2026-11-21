// Package port owns the typed outcomes and the read-only observations behind
// checkout socket backend detection. It performs no I/O.
package port

import "context"

// Backend is the kernel mechanism that keeps late host sockets away from the
// worker. The zero value is Unselected, which never certifies that no mechanism
// exists: only Neither does, and only a complete detection returns it.
type Backend uint8

const (
	Unselected Backend = iota
	// Landlock restricts pathname sockets from inside the worker (ABI 9 or later).
	Landlock
	// GVisor is the daemon's runsc runtime, registered by path with the default
	// host-uds policy of none.
	GVisor
	// Neither confirms that both lookups completed and found no backend.
	Neither
)

// Failure is a typed inspection failure. It carries no raw error or daemon text.
type Failure uint8

const (
	None Failure = iota
	// Unsupported refuses a platform other than the supported one before any I/O.
	Unsupported
	// Cancelled ends a call whose context ended, before or during observation.
	Cancelled
	// NoProbe reports a detector without the observations it needs.
	NoProbe
	// ABIQuery is a kernel query error other than the two absence errnos.
	ABIQuery
	// ABIInvalid is a successful kernel reply that is not a usable ABI.
	ABIInvalid
	// RuntimeInspect covers an unreachable or untrusted daemon and an unusable response.
	RuntimeInspect
	// RuntimeRegistration is a runsc entry whose identity or arguments the
	// detector cannot establish as gVisor with the host-uds policy none.
	RuntimeRegistration
)

// Selection is one detection's outcome. A failure never carries a backend, and
// the zero value confirms nothing.
type Selection struct {
	Backend Backend
	Failure Failure
}

// Confirmed reports a complete detection that reached a backend or confirmed
// that none is available.
func (s Selection) Confirmed() bool { return s.Failure == None && s.Backend != Unselected }

// Platform is the host's operating system and architecture, supplied by assembly.
type Platform struct{ OS, Architecture string }

// ABI is the kernel's answer to the Landlock version query. The zero value is
// not an answer: a present Landlock has a positive Level, and a confirmed
// absence sets Absent with a zero Level.
type ABI struct {
	Level  int
	Absent bool
}

// Kernel asks the running kernel for its highest Landlock ABI, afresh on each
// call. A cancelled context fails with Cancelled before any query.
type Kernel interface {
	LandlockABI(ctx context.Context) (ABI, Failure)
}

// Registration is what the daemon reports for the runtime named runsc. The zero
// value is not an answer.
type Registration uint8

const (
	RegistrationUnknown Registration = iota
	// RunscAbsent confirms a reported inventory without a runtime named runsc.
	RunscAbsent
	// RunscRegistered confirms a path-based runsc registration whose arguments
	// leave host sockets unreachable.
	RunscRegistered
)

// Runtimes reads the daemon's runtime registrations, afresh on each call. It
// never starts or changes anything. A cancelled context fails with Cancelled
// before any request.
type Runtimes interface {
	Runsc(ctx context.Context) (Registration, Failure)
}
