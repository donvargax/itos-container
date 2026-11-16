package features

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type runProcess struct {
	approval map[string]runEntry
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
	out, err strings.Builder
	start    time.Time
	count    int64
	coverage string
}

// Native fixtures keep an oracle-only doctor API with no listening socket.
// Its real requests are captured by the forwarding recorder, not this counter.
func (w *world) runConnectionCount() int64 {
	if w.doctor.engine.listener == nil {
		return 0
	}
	return w.doctor.engine.listener.connections.Load()
}
