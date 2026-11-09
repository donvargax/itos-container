// Package command carries approve's options into its domain.
package command

import (
	"github.com/donvargax/itos-container/internal/cli"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/slices/approve/domain"
	"github.com/donvargax/itos-container/internal/slices/approve/port"
)

// CLI offers terminal review or exact optional fingerprint consent.
type CLI struct {
	JSON         bool                  `name:"json" negatable:"" env:"ITOS_JSON" help:"Print one JSON object; never prompt."`
	Fingerprint  *string               `help:"Approve these exact current settings without a terminal, only for offline grants without credentials."`
	Service      port.Service          `kong:"-"`
	Presentation approval.Presentation `kong:"-"`
}

// Run delegates consent and presentation through their ports.
func (c CLI) Run(_ *cli.UI) int {
	return c.Presentation.Print(domain.Approve(c.Service, c.Presentation, c.Fingerprint, c.JSON), c.JSON)
}

// Help states both the human flow and its deliberate host-agent limitation.
func (CLI) Help() string {
	return "\nBare approve at a terminal shows effective settings and changes since the last approval, then asks y/N.\nApproval is remembered until settings change; an unchanged approval does not ask again.\nUse doctor to inspect the fingerprint. Optional --fingerprint <value> approves only a matching offline grant without credentials and without a terminal.\nNetwork or credential grants require the person at a real terminal; --fingerprint refuses. Bare approve without terminal stdin and stdout, or with JSON, refuses.\n\nan agent on the host can approve through a faked terminal while unsandboxed.\nApproval records what was approved, not who.\n"
}
