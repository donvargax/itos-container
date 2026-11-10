package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (c finiteConnection) CreateNetwork(ctx context.Context, r port.Record, k port.NetworkKind) (string, port.Outcome) {
	type result struct {
		id string
		f  port.Outcome
	}
	v, err := connectionCall(ctx, c.gate, "network creation", func(call context.Context) result {
		id, f := c.RunConnection.CreateNetwork(call, r, k)
		return result{id, f}
	})
	if err != nil {
		return "", port.Uncertain
	}
	return v.id, v.f
}
func (c finiteConnection) InspectNetwork(ctx context.Context, id string) (port.NetworkObservation, port.Inspection) {
	type result struct {
		o port.NetworkObservation
		s port.Inspection
	}
	v, err := connectionCall(ctx, c.gate, "network inspection", func(call context.Context) result {
		o, s := c.RunConnection.InspectNetwork(call, id)
		return result{o, s}
	})
	if err != nil {
		return port.NetworkObservation{}, port.Unavailable
	}
	return v.o, v.s
}
func (c finiteConnection) RemoveNetwork(ctx context.Context, id string) {
	_, _ = connectionCall(ctx, c.gate, "network removal", func(call context.Context) bool { c.RunConnection.RemoveNetwork(call, id); return true })
}
func (c finiteConnection) CreateProxy(ctx context.Context, r port.Record) (string, port.Outcome) {
	type result struct {
		id string
		f  port.Outcome
	}
	v, err := connectionCall(ctx, c.gate, "proxy creation", func(call context.Context) result { id, f := c.RunConnection.CreateProxy(call, r); return result{id, f} })
	if err != nil {
		return "", port.Uncertain
	}
	return v.id, v.f
}
func (c finiteConnection) VerifyProxy(ctx context.Context, id string) port.Outcome {
	f, err := connectionCall(ctx, c.gate, "proxy verification", func(call context.Context) port.Outcome { return c.RunConnection.VerifyProxy(call, id) })
	if err != nil {
		return port.IsolationInspection
	}
	return f
}
func (c finiteConnection) ProxyReady(ctx context.Context, id string) port.Outcome {
	f, err := connectionCall(ctx, c.gate, "proxy readiness", func(call context.Context) port.Outcome { return c.RunConnection.ProxyReady(call, id) })
	if err != nil {
		return port.ProxyFailed
	}
	return f
}
func (c finiteConnection) CopyProxyLog(ctx context.Context, id string, w port.Writer) port.Outcome {
	f, err := connectionCall(ctx, c.gate, "proxy log copy", func(call context.Context) port.Outcome { return c.RunConnection.CopyProxyLog(call, id, w) })
	if err != nil {
		return port.Incomplete
	}
	return f
}
