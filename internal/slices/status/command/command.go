// Package command is the thin status CLI boundary.
package command

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/status/domain"
)

type CLI struct {
	Context      context.Context           `kong:"-"`
	Service      port.Recovery             `kong:"-"`
	Presentation port.RecoveryPresentation `kong:"-"`
	JSON         bool                      `name:"json" negatable:"" help:"Print one schema-1 JSON object."`
	Args         []string                  `arg:"" optional:"" sep:"none" help:"No positional arguments are accepted."`
}

func (CLI) Help() string {
	return "\nUse itos container status with no positional arguments.\n" +
		"Inspect only the effective project and current host user's sessions.\n" +
		"--json or ITOS_JSON=1 selects JSON; --no-json overrides the environment.\n" +
		"An absent controller makes incomplete cleanup a stop candidate; it never authorizes restart.\n" +
		"An unknown workload exit remains unknown. Stop confirms cleanup, not workload success.\n" +
		"Exit 0 means complete inspection, 1 refused ownership/unsafe state, 2 usage/invalid state,\n" +
		"3 unavailable environment, 70 internal failure, 75 temporary uncertainty.\n"
}
func (c CLI) Run() int {
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return c.Presentation.Print(domain.Inspect(ctx, c.Service, c.Args), c.JSON)
}
