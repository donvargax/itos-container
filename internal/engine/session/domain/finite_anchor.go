package domain

import (
	"context"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (w finiteWorkspace) HoldProcess(pid int) (port.Process, port.Outcome) {
	a, ok := w.Workspace.(port.ProcessWorkspace)
	if !ok {
		return nil, port.MountRefused
	}
	type result struct {
		process port.Process
		failure port.Outcome
	}
	r, err := budget.Acquire(w.ctx, "worker process reference", func(call context.Context) result {
		select {
		case <-w.gate:
			defer func() { w.gate <- struct{}{} }()
			if budget.Cause(call) != nil {
				return result{failure: port.MountRefused}
			}
			p, f := a.HoldProcess(pid)
			return result{p, f}
		case <-call.Done():
			return result{failure: port.MountRefused}
		}
	}, func(r result) {
		if r.process != nil {
			r.process.Close()
		}
	})
	if err != nil {
		return nil, port.MountRefused
	}
	if r.process == nil {
		return nil, r.failure
	}
	return finiteProcess{r.process, w.ctx, w.gate}, r.failure
}

type finiteProcess struct {
	port.Process
	ctx  context.Context
	gate chan struct{}
}

func (p finiteProcess) Exited() (bool, port.Outcome) {
	type result struct {
		exited  bool
		failure port.Outcome
	}
	r, err := held(p.ctx, p.gate, "worker process observation", func() result { e, f := p.Process.Exited(); return result{e, f} })
	if err != nil {
		return false, port.MountRefused
	}
	return r.exited, r.failure
}
func (p finiteProcess) Close() {
	select {
	case <-p.gate:
		p.Process.Close()
		p.gate <- struct{}{}
	default:
		go func() { <-p.gate; p.Process.Close(); p.gate <- struct{}{} }()
	}
}

func (w finiteWorkspace) Source() (port.MountSource, port.Outcome) {
	return w.anchorCall("checkout descriptor", func(a port.AnchorWorkspace) (port.MountSource, port.Outcome) { return a.Source() })
}
func (w finiteWorkspace) Mounted(pid int) (port.MountSource, port.Outcome) {
	return w.anchorCall("mounted checkout identity", func(a port.AnchorWorkspace) (port.MountSource, port.Outcome) { return a.Mounted(pid) })
}
func (w finiteWorkspace) anchorCall(name string, fn func(port.AnchorWorkspace) (port.MountSource, port.Outcome)) (port.MountSource, port.Outcome) {
	a, ok := w.Workspace.(port.AnchorWorkspace)
	if !ok {
		return port.MountSource{}, port.MountUnavailable
	}
	type result struct {
		source  port.MountSource
		failure port.Outcome
	}
	r, err := held(w.ctx, w.gate, name, func() result { s, f := fn(a); return result{s, f} })
	if err != nil {
		return port.MountSource{}, port.MountRefused
	}
	return r.source, r.failure
}
func (c finiteConnection) AllocateAnchor(ctx context.Context, r port.Record) port.Outcome {
	f, err := connectionCall(ctx, c.gate, "anchor allocation", func(call context.Context) port.Outcome { return c.RunConnection.AllocateAnchor(call, r) })
	if err != nil {
		return port.MountUncertain
	}
	return f
}
func (c finiteConnection) InspectAnchor(ctx context.Context, name string) (port.AnchorObservation, port.Inspection) {
	type result struct {
		o      port.AnchorObservation
		status port.Inspection
	}
	r, err := connectionCall(ctx, c.gate, "anchor inspection", func(call context.Context) result {
		o, s := c.RunConnection.InspectAnchor(call, name)
		return result{o, s}
	})
	if err != nil {
		return port.AnchorObservation{}, port.Unavailable
	}
	return r.o, r.status
}
func (c finiteConnection) RemoveAnchor(ctx context.Context, name string) {
	_, _ = connectionCall(ctx, c.gate, "anchor removal", func(call context.Context) bool { c.RunConnection.RemoveAnchor(call, name); return true })
}
