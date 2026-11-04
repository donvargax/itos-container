// Package command carries doctor's command-line request to its domain.
package command

import (
	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/slices/doctor/domain"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// CLI is the doctor command, with its inspection capability injected by main.
type CLI struct {
	Policies   port.Policies `kong:"-"`
	PolicyOnly bool          `name:"policy-only" negatable:"" help:"Validate only itos-container.yaml, without manifest, host or approval checks."`
	JSON       bool          `name:"json" negatable:"" env:"ITOS_JSON" help:"Print one JSON object."`
}

// Help documents the implemented inspection scope and machine contract.
func (CLI) Help() string {
	return "\nUse --policy-only to validate itos-container.yaml in the effective working directory.\n\n" +
		"    --json prints schema 1, ok, scope policy, problems and the effective policy.\n" +
		"    Host readiness and approval are not implemented in this slice.\n" +
		"    Exit codes:\n" +
		"      0  policy validation succeeded\n" +
		"      2  invalid policy or command line\n" +
		"      3  policy cannot be read\n"
}

// Run reports the domain's inspection result through the UI.
func (c CLI) Run(ui *cli.UI) int { return presentPolicy(ui, domain.Inspect(c.Policies), c.JSON) }
