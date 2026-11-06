package port

import "context"

const (
	IsolationUnverified Outcome = "isolation-unverified"
	IsolationInspection Outcome = "isolation-inspection"
	ExecutionFailed     Outcome = "execution-unavailable"
	EnvironmentMissing  Outcome = "engine-environment-missing"
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
	Result
	Exit   int
	Exited bool
}

// Workspace pins the approved subdirectory and refuses later substitution.
type Workspace interface {
	Pin(Binding) Outcome
	Unchanged() bool
	Close()
}

// RunEngine is deliberately separate from slice-9's unstarted-only capability.
type RunEngine interface {
	OpenRun(context.Context, Binding) (RunConnection, Outcome)
}
type RunConnection interface {
	Connection
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
