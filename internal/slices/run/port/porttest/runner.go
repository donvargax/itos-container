package porttest

import (
	"context"
	"slices"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
)

type Runner struct {
	Result              session.RunResult
	Frozen              approval.Durable
	Argv                []string
	Output, ErrorOutput []byte
}

func (r *Runner) Run(_ context.Context, frozen approval.Durable, argv []string, streams session.Streams) session.RunResult {
	r.Frozen, r.Argv = frozen, slices.Clone(argv)
	if streams.Out != nil {
		_, _ = streams.Out.Write(r.Output)
	}
	if streams.Err != nil {
		_, _ = streams.Err.Write(r.ErrorOutput)
	}
	return r.Result
}
