package domain

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

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
