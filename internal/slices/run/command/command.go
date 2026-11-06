// Package command translates run's invocation and refusals at the CLI boundary.
package command

import (
	"context"
	"errors"
	approvalport "github.com/donvargax/itos-container/internal/engine/approval/port"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/run/port"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/slices/run/domain"
)

// CLI lets Kong retain the first separator and every subsequent literal word.
type CLI struct {
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
		"    Exit codes:\n" +
		"      2  missing command/separator or unsupported JSON request\n" +
		"      1  approval missing, changed, invalid or superseded; use itos container approve\n" +
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
	frozen, err := domain.Preflight(c.Approval, c.Snapshots)
	if err != nil {
		return ui.Fail(err, false)
	}
	result := c.Runner.Run(context.Background(), frozen, args, session.Streams{In: ui.In, Out: ui.Stdout, Err: ui.Stderr})
	code := 0
	if result.Outcome != "" {
		code = ui.Fail(runFailure{result.Outcome, result.Reference, false}, false)
	}
	if result.Reference != "" && result.Cleanup != session.Complete {
		cleanupCode := ui.Fail(runFailure{result.Cleanup, result.Reference, true}, false)
		if code == 0 {
			code = cleanupCode
		}
	}
	if result.Exited && result.Exit != 0 {
		return result.Exit
	}
	return code
}

type runFailure struct {
	outcome   session.Outcome
	reference string
	cleanup   bool
}

func (runFailure) Error() string { return "owned run could not be completed" }

// Classify supplies run's stable rules and human guidance to the generic UI.
func Classify(err error) (int, []cli.Problem) {
	var run runFailure
	if errors.As(err, &run) {
		code, cause := 75, "temporary"
		switch run.outcome {
		case session.Refused, session.Unsafe:
			code, cause = 1, "ownership"
		case session.Invalid:
			code, cause = 2, "configuration"
		case session.IdentityFailed:
			code, cause = 70, "internal"
		case session.IsolationUnverified:
			code = 1
		}
		rule := "run." + string(run.outcome)
		if run.cleanup {
			rule = "run.cleanup-" + cause
		}
		fix := "Have the host user inspect the protected intent and original engine; cleanup is not confirmed."
		return code, []cli.Problem{{Rule: rule, Exit: code, Message: rule + " " + strconv.QuoteToASCII(run.reference) + ": " + run.Error(), Fix: fix}}
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
