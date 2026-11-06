package port

import (
	"context"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
)

type Runner interface {
	Run(context.Context, approval.Durable, []string, session.Streams) session.RunResult
}
