// Package command is the thin stop CLI boundary.
package command

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/stop/domain"
)

type CLI struct {
	Context      context.Context           `kong:"-"`
	Service      port.Recovery             `kong:"-"`
	Presentation port.RecoveryPresentation `kong:"-"`
	JSON         bool                      `name:"json" negatable:"" env:"ITOS_JSON" help:"Print one schema-1 JSON object."`
	Args         []string                  `arg:"" optional:"" sep:"none" help:"Exactly one full 64-digit lowercase hexadecimal session ID."`
}

func (CLI) Help() string {
	return "\nUse itos container stop <session-id>, exactly one full 64-digit lowercase hexadecimal ID.\n" +
		"Stop only this effective project and current host user's registered session.\n" +
		"--json or ITOS_JSON=1 selects JSON; --no-json overrides the environment.\n" +
		"Exit 0 confirms durable cleanup, not successful workload execution; an unknown exit stays unknown.\n" +
		"An absent controller never authorizes restart. Explicit stop also works with a present controller.\n" +
		"Exit 1 means refused ownership/unsafe state, 2 usage/invalid state, 3 unavailable environment,\n" +
		"70 internal failure, 75 temporary uncertainty; retry in the same effective project.\n"
}
func (c CLI) Run() int {
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return c.Presentation.Print(domain.Stop(ctx, c.Service, c.Args), c.JSON)
}
