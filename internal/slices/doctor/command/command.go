// Package command carries doctor's command-line request to its domain.
package command

import (
	"fmt"
	"github.com/donvargax/itos-container/internal/cli"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/domain"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// CLI is the doctor command, with its inspection capability injected by main.
type CLI struct {
	Approval     approval.Inspector    `kong:"-"`
	Presentation approval.Presentation `kong:"-"`
	Policies     port.Policies         `kong:"-"`
	Manifests    port.Manifests        `kong:"-"`
	Readiness    port.Readiness        `kong:"-"`
	PolicyOnly   bool                  `name:"policy-only" negatable:"" help:"Validate only itos-container.yaml, without manifest, host or approval checks."`
	ManifestOnly bool                  `name:"manifest-only" negatable:"" help:"Validate policy and manifest only; image metadata, host readiness and approval are not checked."`
	JSON         bool                  `name:"json" negatable:"" env:"ITOS_JSON" help:"Print one JSON object."`
}

// Help documents the implemented inspection scope and machine contract.
func (CLI) Help() string {
	return "\nUse --policy-only to validate itos-container.yaml in the effective working directory.\n" +
		"Use --manifest-only to validate the selected manifest and checkout workspace.\n" +
		"Image metadata, host readiness and approval were not checked by these modes.\n\n" +
		"    Ordinary doctor inspects the checkout, local engine and exact cached image.\n" +
		"    --json prints schema 1, ok, problems, effective data and inspection facts.\n" +
		"    File-only modes identify their scope as policy or manifest.\n" +
		"    Ordinary inspection reports the fingerprint and current approval status; runtime isolation remains unproven.\n" +
		"    Exit codes:\n" +
		"      0  policy validation succeeded; other requested inspections also succeeded\n" +
		"      1  unsupported or changed checkout, host, engine or image, or control state that is\n" +
		"         unsafe, invalid or exposed to a worker checkout\n" +
		"      2  invalid policy or command line, or invalid selected manifest, or a state location that\n" +
		"         is relative or not valid UTF-8 (approval.state.location)\n" +
		"      3  policy cannot be read, or another required local resource is unavailable,\n" +
		"         including the account home or retained control state\n" +
		"      75 engine inspection failed or exceeded five seconds, or the control-state lock\n" +
		"         was still held after ten seconds (approval.state.busy; retry the same command)\n"
}

// Run reports the domain's inspection result through the UI.
func (c CLI) Run(ui *cli.UI) int {
	if c.PolicyOnly && c.ManifestOnly {
		return ui.Usage("doctor", fmt.Errorf("--manifest-only and --policy-only cannot be combined"), c.JSON)
	}
	if c.PolicyOnly {
		return presentPolicy(ui, domain.Inspect(c.Policies), c.JSON)
	}
	if !c.ManifestOnly {
		if c.Approval != nil {
			view := c.Approval.Inspect()
			return presentApprovalReadiness(ui, view, c.Presentation, c.JSON)
		}
		return presentReadiness(ui, domain.InspectReadiness(c.Readiness), c.JSON)
	}
	policy, manifest := domain.InspectManifest(c.Policies, c.Manifests)
	if manifest == nil {
		return presentPolicy(ui, policy, c.JSON)
	}
	return PresentManifest(ui, *manifest, c.JSON)
}
