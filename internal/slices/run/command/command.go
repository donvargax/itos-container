// Package command translates run's invocation and refusals at the CLI boundary.
package command

import (
	"errors"
	approvalport "github.com/donvargax/itos-container/internal/engine/approval/port"
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
	JSON          bool                   `name:"json" negatable:"" help:"Request JSON (refused by run)."`
	Args          []string               `arg:"" optional:"" passthrough:"partial" sep:"none" help:"-- followed by a nonempty executable and its arguments."`
	InheritedJSON bool                   `kong:"-"`
}

// Help makes the interim refusal explicit without claiming workload execution.
func (CLI) Help() string {
	return "\nUse itos container run -- <command> [arguments...].\n\n" +
		"    Wrapper --json or ITOS_JSON=1 returns one schema-1 refusal object.\n" +
		"    Arguments after -- belong only to the workload.\n" +
		"    Offline startup is not implemented; doctor --policy-only inspects policy only.\n" +
		"    Exit codes:\n" +
		"      2  missing command/separator or unsupported JSON request\n" +
		"      1  approval missing, changed, invalid or superseded; use itos container approve\n" +
		"      3  offline startup is not implemented\n"
}

// Run delegates invocation validation without assembling any startup adapters.
func (c CLI) Run(ui *cli.UI) int {
	args := c.Args
	separated := len(args) > 0 && args[0] == "--"
	if separated {
		args = args[1:]
	}
	withJSON := c.JSON || c.InheritedJSON
	failure := domain.Refuse(domain.Invocation{JSON: withJSON, Separated: separated, Argv: args})
	if errors.Is(failure, domain.ErrStartupUnavailable) && c.Approval != nil {
		if _, err := domain.Preflight(c.Approval, c.Snapshots); err != nil {
			return ui.Fail(err, withJSON)
		}
		ui.Line("approval matched")
	}
	return ui.Fail(failure, withJSON)
}

// Classify supplies run's stable rules and human guidance to the generic UI.
func Classify(err error) (int, []cli.Problem) {
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
	case errors.Is(err, domain.ErrStartupUnavailable):
		return cli.CodeEnvironment, []cli.Problem{{
			Rule: "run.startup-unavailable", Exit: cli.CodeEnvironment,
			Message: "offline startup is not implemented; doctor --policy-only inspects policy only",
			Fix:     "Use itos container doctor --policy-only for policy inspection; it does not authorize or start a run.",
		}}
	default:
		return 0, nil
	}
}
