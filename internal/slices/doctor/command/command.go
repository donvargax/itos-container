// Package command carries doctor's command-line request to its domain.
package command

import (
	"fmt"
	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/slices/doctor/domain"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// CLI is the doctor command, with its inspection capability injected by main.
type CLI struct {
	Policies     port.Policies  `kong:"-"`
	Manifests    port.Manifests `kong:"-"`
	PolicyOnly   bool           `name:"policy-only" negatable:"" help:"Validate only itos-container.yaml, without manifest, host or approval checks."`
	ManifestOnly bool           `name:"manifest-only" negatable:"" help:"Validate policy and manifest only; image metadata, host readiness and approval are not checked."`
	JSON         bool           `name:"json" negatable:"" env:"ITOS_JSON" help:"Print one JSON object."`
}

// Help documents the implemented inspection scope and machine contract.
func (CLI) Help() string {
	return "\nUse --policy-only to validate itos-container.yaml in the effective working directory.\n" +
		"Use --manifest-only to validate the selected manifest and checkout workspace.\n" +
		"Image metadata, host readiness and approval were not checked by these modes.\n\n" +
		"    --json prints schema 1, ok, scope policy or manifest, problems and effective data.\n" +
		"    Host readiness and approval are not implemented in this slice.\n" +
		"    Exit codes:\n" +
		"      0  policy validation succeeded; selected manifest also valid unless --policy-only\n" +
		"      2  invalid policy or command line, or invalid selected manifest\n" +
		"      3  policy cannot be read\n"
}

// Run reports the domain's inspection result through the UI.
func (c CLI) Run(ui *cli.UI) int {
	if c.PolicyOnly && c.ManifestOnly {
		return ui.Usage("doctor", fmt.Errorf("--manifest-only and --policy-only cannot be combined"), c.JSON)
	}
	if c.PolicyOnly {
		return presentPolicy(ui, domain.Inspect(c.Policies), c.JSON)
	}
	policy, manifest := domain.InspectManifest(c.Policies, c.Manifests)
	if manifest == nil {
		return presentPolicy(ui, policy, c.JSON)
	}
	return PresentManifest(ui, *manifest, c.JSON)
}
