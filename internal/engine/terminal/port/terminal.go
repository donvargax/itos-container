// Package port defines an explicitly selected host terminal, independent of streams
// and container sessions. A provider must have exclusive ownership of its terminal:
// callers must not bind the same terminal through several providers, close its file,
// or change its settings while a lease is active.
package port

import "context"

// Dimensions are measured from the selected terminal, never from the environment.
type Dimensions struct{ Columns, Rows int }

// Valid reports whether both dimensions fit the terminal's unsigned 16-bit size.
func (d Dimensions) Valid() bool {
	return d.Columns >= 1 && d.Columns <= 65535 && d.Rows >= 1 && d.Rows <= 65535
}

// FailureKind describes an acquisition refusal. No usable lease accompanies one.
type FailureKind string

const (
	Unsupported       FailureKind = "unsupported"
	InvalidTerminal   FailureKind = "invalid-terminal"
	InvalidDimensions FailureKind = "invalid-dimensions"
	Cancelled         FailureKind = "cancelled"
	Busy              FailureKind = "busy"
	AcquisitionFailed FailureKind = "acquisition-failed"
)

// Failure retains the underlying acquisition error without exposing terminal data.
type Failure struct {
	Kind FailureKind
	Err  error
}

func (f *Failure) Error() string { return "terminal: " + string(f.Kind) }
func (f *Failure) Unwrap() error { return f.Err }

// Cause is the first observed reason a lease ended, independently of restoration.
type Cause string

const (
	Closed            Cause = "closed"
	ContextCancelled  Cause = "context-cancelled"
	Hangup            Cause = "hangup"
	ObservationFailed Cause = "observation-failed"
)

// Result is retained unchanged after termination. RestoreErr being nil means the
// saved state was restored; a vanished terminal may make that impossible.
type Result struct {
	Cause      Cause
	Err        error
	RestoreErr error
}

// Provider acquires raw mode and initial dimensions atomically from the caller's
// perspective. Overlap on one provider refuses as Busy; release permits reuse.
type Provider interface {
	Acquire(context.Context) (Lease, error)
}

// Lease owns settings and observation, never the caller's file or payload bytes.
// Resizes has bounded storage and may coalesce to the latest dimensions. Done
// closes only after observation ends and restoration is attempted; Resizes also
// closes. Result waits for Done. Close requests termination, waits for cleanup,
// and returns the same retained result on every call, including concurrent calls.
type Lease interface {
	Initial() Dimensions
	Resizes() <-chan Dimensions
	Done() <-chan struct{}
	Result() Result
	Close() Result
}
