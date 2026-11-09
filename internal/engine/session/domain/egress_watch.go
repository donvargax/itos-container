package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (s Runner) waitEgress(ctx context.Context, c port.RunConnection, a port.Attachment, b port.Binding, r port.Record) (int, bool, port.Outcome) {
	if b.Egress == (port.EgressGrant{}) {
		return s.waitRun(ctx, c, a, r.Anchor.Session, r.ContainerID)
	}
	wait, cancel := context.WithCancel(ctx)
	defer cancel()
	watcher, f := (finiteEngine{s.Engine}).OpenRun(wait, b)
	if f != "" {
		return 0, false, f
	}
	defer watcher.Close()
	type result struct {
		exit    int
		exited  bool
		failure port.Outcome
	}
	worker := make(chan result, 1)
	proxy := make(chan port.Outcome, 1)
	go func() {
		exit, exited, f := s.waitRun(wait, c, a, r.Anchor.Session, r.ContainerID)
		worker <- result{exit, exited, f}
	}()
	go func() { _, f := watcher.Wait(wait, r.Egress.Proxy.ID); proxy <- f }()
	select {
	case v := <-worker:
		return v.exit, v.exited, v.failure
	case f := <-proxy:
		// An already observed worker exit wins over a simultaneous proxy result.
		select {
		case v := <-worker:
			return v.exit, v.exited, v.failure
		default:
		}
		cancel()
		v := <-worker
		if f == "" {
			f = port.ProxyExited
		}
		return v.exit, v.exited, f
	}
}
