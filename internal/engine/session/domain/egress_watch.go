package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (s Runner) waitEgress(ctx context.Context, c port.RunConnection, a port.Attachment, b port.Binding, r port.Record) (int, bool, port.Outcome) {
	return s.waitRun(ctx, c, a, r.Anchor.Session, r.ContainerID)
}
