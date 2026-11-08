// Package command translates run's invocation and refusals at the CLI boundary.
package command

import (
	"context"
	"errors"
	approvalport "github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/run/port"
	"strconv"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/slices/run/domain"
)

var errWorkloadExitUnknown = errors.New("workload exit is unknown; cleanup is complete")

// CLI lets Kong retain the first separator and every subsequent literal word.
type CLI struct {
	Context       context.Context        `kong:"-"`
	Snapshots     approvalport.Snapshots `kong:"-"`
	Approval      port.Approval          `kong:"-"`
	Runner        port.Runner            `kong:"-"`
	JSON          bool                   `name:"json" negatable:"" help:"Request JSON (refused by run)."`
	Args          []string               `arg:"" optional:"" passthrough:"partial" sep:"none" help:"-- followed by a nonempty executable and its arguments."`
	InheritedJSON bool                   `kong:"-"`
}

// Help describes foreground execution and the wrapper/workload boundary.
func (CLI) Help() string {
	return "\nUse itos container run -- <command> [arguments...].\n\n" +
		"    Wrapper --json or ITOS_JSON=1 returns one schema-1 refusal object.\n" +
		"    Arguments after -- belong only to the workload.\n" +
		"    Run the literal command in one approved offline worker; streams remain separate.\n" +
		"    The worker uses the frozen workspace, host UID:GID and temporary HOME/TMPDIR.\n" +
		"    A Unix socket anywhere in the checkout, including .git, refuses startup before worker creation.\n" +
		"    Host sockets created or swapped into the checkout after inspection remain reachable during a run.\n" +
		"    Such sockets can expose host actions or networking despite the offline network namespace.\n" +
		"    Exit codes:\n" +
		"      2  missing command/separator or unsupported JSON request\n" +
		"      1  approval missing, changed, invalid or superseded; use itos container approve\n" +
		"      1  isolation unverified, including a checkout Unix socket\n" +
		"      3  required local environment unavailable\n" +
		"      75 temporary controller or cleanup failure\n" +
		"    An observed nonzero workload exit wins over a separate cleanup failure.\n"
}

// Run validates before preflight, then delegates the owned foreground lifecycle.
func (c CLI) Run(ui *cli.UI) int {
	args := c.Args
	separated := len(args) > 0 && args[0] == "--"
	if separated {
		args = args[1:]
	}
	withJSON := c.JSON || c.InheritedJSON
	failure := domain.Refuse(domain.Invocation{JSON: withJSON, Separated: separated, Argv: args})
	if failure != nil {
		return ui.Fail(failure, withJSON)
	}
	if c.Approval == nil || c.Snapshots == nil || c.Runner == nil {
		return ui.Fail(errors.New("run assembly is incomplete"), false)
	}
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := context.Cause(ctx).(session.Interrupt); ok {
		return reportRun(ui, session.RunResult{Signal: context.Cause(ctx).(session.Interrupt).Signal, Deadline: time.Now().Add(30 * time.Second)})
	}
	startup, end := budget.Start(ctx, 120*time.Second, "startup")
	defer end()
	frozen, err := domain.PreflightContext(startup, c.Approval, c.Snapshots)
	if err != nil {
		if interrupted, ok := context.Cause(ctx).(session.Interrupt); ok {
			return reportRun(ui, session.RunResult{Signal: interrupted.Signal, Deadline: time.Now().Add(30 * time.Second)})
		}
		var timed budget.Failure
		if errors.As(err, &timed) {
			outcome := session.CallTimeout
			if timed.Phase != "" {
				outcome = session.StartupTimeout
			}
			return reportRun(ui, session.RunResult{Result: session.Result{Outcome: outcome}, Operation: timed.Operation, Deadline: time.Now().Add(30 * time.Second)})
		}
		return ui.Fail(err, false)
	}
	if interrupted, ok := context.Cause(ctx).(session.Interrupt); ok {
		return reportRun(ui, session.RunResult{Signal: interrupted.Signal, Deadline: time.Now().Add(30 * time.Second)})
	}
	result := c.Runner.Run(budget.WithStartup(ctx, startup), frozen, args, session.Streams{In: ui.In, Out: ui.Stdout, Err: ui.Stderr})
	return reportRun(ui, result)
}

func reportRun(ui *cli.UI, result session.RunResult) int {
	code := 0
	if result.Outcome != "" {
		code, _ = Classify(runFailure{result.Outcome, result.Reference, result.SessionID, false})
	}
	if result.Teardown != "" && code == 0 {
		code, _ = Classify(runFailure{result.Teardown, result.Reference, result.SessionID, false})
	}
	if result.Stream != "" && code == 0 {
		code, _ = Classify(runFailure{result.Stream, result.Reference, result.SessionID, false})
	}
	if result.Reference != "" && result.Cleanup != session.Complete && code == 0 {
		code, _ = Classify(runFailure{result.Cleanup, result.Reference, result.SessionID, true})
	}
	if result.Signal != "" && !result.Exited {
		code = 130
		if result.Signal == "SIGTERM" {
			code = 143
		}
	}
	if result.Exited && result.Exit != 0 {
		code = result.Exit
	}
	unknownExit := code == 0 && !result.Exited
	if unknownExit {
		code, _ = Classify(errWorkloadExitUnknown)
	}
	// Diagnostic output is a finite external operation within teardown. Keep
	// its ten-second call limit even when cleanup leaves more time available.
	// An abandoned caller-owned writer is neither closed nor joined.
	ctx := context.Background()
	if !result.Deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, result.Deadline)
		defer cancel()
	}
	_, _ = budget.Call(ctx, "diagnostic output", func(context.Context) struct{} {
		if result.Signal != "" {
			ui.Fail(cancelFailure{result.Signal, workloadOutcome(result)}, false)
		}
		if result.Stream != "" {
			ui.Fail(runFailure{result.Stream, result.Reference, result.SessionID, false}, false)
		}
		if result.Outcome != "" {
			failure := runFailure{result.Outcome, result.Reference, result.SessionID, false}
			if result.Operation != "" {
				ui.Fail(operationFailure{failure, result.Operation}, false)
			} else {
				ui.Fail(failure, false)
			}
		}
		if result.Teardown != "" {
			ui.Fail(runFailure{result.Teardown, result.Reference, result.SessionID, false}, false)
		}
		if result.Reference != "" && result.Cleanup != session.Complete {
			ui.Fail(runFailure{result.Cleanup, result.Reference, result.SessionID, true}, false)
		}
		if unknownExit {
			ui.Fail(errWorkloadExitUnknown, false)
		}
		return struct{}{}
	})
	return code
}

func workloadOutcome(r session.RunResult) string {
	if r.Exited {
		return strconv.Itoa(r.Exit)
	}
	if !r.Started {
		return "not-started"
	}
	return "unknown"
}

type operationFailure struct {
	runFailure
	operation string
}

func (f operationFailure) Unwrap() error { return f.runFailure }

type cancelFailure struct{ signal, workload string }

func (cancelFailure) Error() string { return "run cancelled" }

type runFailure struct {
	outcome   session.Outcome
	reference string
	sessionID string
	cleanup   bool
}

func (runFailure) Error() string { return "owned run could not be completed" }

// Classify supplies run's stable rules and human guidance to the generic UI.
func Classify(err error) (int, []cli.Problem) {
	var timed budget.Failure
	if errors.As(err, &timed) {
		rule := "run.call-timeout"
		if timed.Phase != "" {
			rule = "run." + timed.Phase + "-timeout"
		}
		return 75, []cli.Problem{{Rule: rule, Exit: 75, Message: rule + "; operation " + timed.Operation, Fix: "Inspect protected recovery state before retrying."}}
	}
	var cancelled cancelFailure
	if errors.As(err, &cancelled) {
		code := 130
		if cancelled.signal == "SIGTERM" {
			code = 143
		}
		return code, []cli.Problem{{Rule: "run.cancelled", Exit: code, Message: "run.cancelled " + cancelled.signal + "; workload outcome " + cancelled.workload, Fix: "Inspect the retained session if cleanup is incomplete."}}
	}
	var run runFailure
	if errors.As(err, &run) {
		if run.outcome == session.CheckoutOverlap {
			guidance := "Finish the other session, or inspect/stop it from its original checkout."
			return 1, []cli.Problem{{Rule: "run.checkout-overlap", Exit: 1,
				Message: "run.checkout-overlap: this checkout overlaps another unfinished session; " + guidance, Fix: guidance}}
		}
		code, cause := 75, "temporary"
		switch run.outcome {
		case session.Refused, session.Unsafe, session.RegistrationOwnership, session.MountRefused:
			code, cause = 1, "ownership"
		case session.Invalid:
			code, cause = 2, "configuration"
		case session.IdentityFailed, session.RegistrationInternal, session.Internal:
			code, cause = 70, "internal"
		case session.Environment, session.EnvironmentMissing, session.MountUnavailable:
			code, cause = 3, "environment"
		case session.IsolationUnverified:
			code = 1
		case session.IntentWrite, session.OwnershipWrite, session.CompletionWrite,
			session.MountWrite, session.MountUncertain,
			session.Uncertain, session.Incomplete, session.IsolationInspection, session.ExecutionFailed, session.StreamTimeout, session.EngineLost, session.TeardownTimeout:
			// Known transient failures retain their temporary classification.
		default:
			if run.cleanup {
				code, cause = 70, "internal"
			}
		}
		rule := "run." + string(run.outcome)
		if run.cleanup {
			rule = "run.cleanup-" + cause
		}
		fix := "Have the host user inspect the protected intent and original engine; cleanup is not confirmed."
		message := rule + " " + strconv.QuoteToASCII(run.reference) + ": " + run.Error()
		var operation operationFailure
		if errors.As(err, &operation) {
			message += "; operation " + operation.operation
		}
		if run.outcome == session.EngineLost {
			message += "; workload outcome unknown unless separately observed"
		}
		if session.HexID(run.sessionID) {
			fix = "Use the same effective project: itos container stop " + run.sessionID
			message += "; " + fix
		}
		return code, []cli.Problem{{Rule: rule, Exit: code, Message: message, Fix: fix}}
	}
	var approval domain.ApprovalRefusal
	if errors.As(err, &approval) {
		return 1, []cli.Problem{{Rule: "run.approval-" + approval.Reason, Exit: 1, Message: "run.approval-" + approval.Reason + ": " + approval.Error(), Fix: "Review current settings with itos container approve."}}
	}
	var prerequisite domain.Prerequisite
	if errors.As(err, &prerequisite) {
		problems := []cli.Problem{}
		code := 0
		for _, p := range approvalport.Problems(prerequisite.View) {
			subject := p.File + p.Field + p.Image
			if p.Path != nil {
				subject += *p.Path
			}
			problems = append(problems, cli.Problem{Rule: p.Rule, Exit: p.Exit, Message: p.Rule + " " + strconv.QuoteToASCII(subject) + ": " + p.Message + "; " + p.Fix, Fix: p.Fix})
			code = max(code, p.Exit)
		}
		return code, problems
	}
	var snapshot domain.SnapshotRefusal
	if errors.As(err, &snapshot) {
		rule := "run." + string(snapshot.Failure)
		code := 1
		message := "Snapshot state is unsafe"
		fix := "Have the host user inspect protected state outside the checkout; use owned 0700 directories and regular 0600 files without links."
		if strings.HasSuffix(rule, "invalid") {
			message = "Snapshot state is invalid or conflicts with selected settings"
		}
		if strings.HasSuffix(rule, "write") {
			code = 75
			message = "Snapshot publication could not be confirmed durable"
			fix = "Have the host user inspect protected state before retrying the full run preflight."
		}
		return code, []cli.Problem{{Rule: rule, Exit: code, Message: rule + " " + strconv.QuoteToASCII(snapshot.Reference) + ": " + message + "; " + fix, Fix: fix}}
	}
	switch {
	case errors.Is(err, errWorkloadExitUnknown):
		return 75, []cli.Problem{{
			Rule: "run.workload-exit-unknown", Exit: 75,
			Message: "run.workload-exit-unknown: " + errWorkloadExitUnknown.Error(),
		}}
	case errors.Is(err, domain.ErrJSONUnsupported):
		return cli.CodeUsage, []cli.Problem{{
			Rule: "run.json-unsupported", Exit: cli.CodeUsage,
			Message: "run does not support wrapper JSON mode",
			Fix:     "Remove the wrapper --json option and the inherited ITOS_JSON=1 request.",
		}}
	case errors.Is(err, domain.ErrCommandRequired):
		return cli.CodeUsage, []cli.Problem{{
			Rule: "run.command-required", Exit: cli.CodeUsage,
			Message: "run requires a nonempty command after --; use itos container run -- <command>",
			Fix:     "Supply a nonempty executable after --.",
		}}
	default:
		return 0, nil
	}
}
