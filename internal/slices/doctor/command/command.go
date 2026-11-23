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
	Approval        approval.Inspector    `kong:"-"`
	Presentation    approval.Presentation `kong:"-"`
	Policies        port.Policies         `kong:"-"`
	RootPolicies    port.Policies         `kong:"-"` // reads through the root Credentials pins
	Manifests       port.Manifests        `kong:"-"`
	Readiness       port.Readiness        `kong:"-"`
	Credentials     port.Credentials      `kong:"-"`
	Sockets         port.Sockets          `kong:"-"`
	PolicyOnly      bool                  `name:"policy-only" negatable:"" help:"Validate only itos-container.yaml, without manifest, host or approval checks."`
	ManifestOnly    bool                  `name:"manifest-only" negatable:"" help:"Validate policy and manifest only; image metadata, host readiness and approval are not checked."`
	CredentialsOnly bool                  `name:"credentials-only" negatable:"" help:"Inspect source metadata without reading credential contents; does not approve."`
	JSON            bool                  `name:"json" negatable:"" env:"ITOS_JSON" help:"Print one JSON object."`
}

// Help documents the implemented inspection scope and machine contract.
func (CLI) Help() string {
	return ""
}

// Run reports the domain's inspection result through the UI.
func (c CLI) Run(ui *cli.UI) int {
	return 0
}

// Blank references keep the declarations used while the bodies are stubs.
var (
	_ = fmt.Errorf
	_ = domain.Inspect
	_ = requestedCredentials
)
