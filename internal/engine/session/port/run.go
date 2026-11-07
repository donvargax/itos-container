package port

import (
	"context"
	"time"
)

const (
	IsolationUnverified Outcome = "isolation-unverified"
	IsolationInspection Outcome = "isolation-inspection"
	ExecutionFailed     Outcome = "execution-unavailable"
	EnvironmentMissing  Outcome = "engine-environment-missing"
	CallTimeout         Outcome = "call-timeout"
	StartupTimeout      Outcome = "startup-timeout"
	TeardownTimeout     Outcome = "teardown-timeout"
	StreamTimeout       Outcome = "stream-timeout"
	EngineLost          Outcome = "engine-lost"
	// StdinClosed is an attachment notification, not a final run outcome. An
	// engine input write/half-close failed; the worker may already have exited.
	StdinClosed Outcome = "stdin-closed"
)

// Streams carry deliberate workload bytes, never controller diagnostics.
type Reader interface{ Read([]byte) (int, error) }
type Writer interface{ Write([]byte) (int, error) }
type Streams struct {
	In       Reader
	Out, Err Writer
}

type RunResult struct {
	Stream Outcome
	// Teardown retains a cleanup deadline failure separately from the run cause.
	Teardown  Outcome
	Operation string
	Result
	Exit    int
	Exited  bool
	Signal  string
	Started bool
	// Deadline bounds diagnostic delivery as part of the original teardown.
	Deadline time.Time
}

// Workspace pins the approved subdirectory and refuses later substitution.
type Workspace interface {
	Pin(Binding) Outcome
	// Validate returns Refused for a confirmed mismatch and Incomplete when the
	// check could not finish. An interrupted observation is not a substitution.
	Validate() Outcome
	Close()
}

// RunEngine is deliberately separate from slice-9's unstarted-only capability.
type RunEngine interface {
	OpenRun(context.Context, Binding) (RunConnection, Outcome)
}
type RunConnection interface {
	Connection
	AnchorConnection
	CreateRun(context.Context, Record, []string) (string, Outcome)
	VerifyRun(context.Context, string, []string) Outcome
	Attach(context.Context, string, Streams) (Attachment, Outcome)
	Start(context.Context, string) Outcome
	Wait(context.Context, string) (int, Outcome)
	Signal(context.Context, string, string) Outcome
	RemoveRun(context.Context, string)
}
type Attachment interface {
	// Failures reports forwarding errors while Wait is still pending. Successful
	// output EOF is not a failure and must not end a running workload.
	// StdinClosed requires exit reconciliation; source read and output errors
	// report ExecutionFailed and remain failures even after observed exit.
	Failures() <-chan Outcome
	// Drain awaits output completion and reports sticky forwarding failures.
	// An engine stdin closure alone does not invalidate a known worker exit.
	Drain(context.Context) Outcome
	Close()
}

// Interrupt carries the first host signal without exposing OS types to domains.
type Interrupt struct{ Signal string }

func (Interrupt) Error() string { return "foreground interrupted" }
