package domain

import (
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (h Homes) list(ctx context.Context) ([]string, port.Outcome) {
	ids, f, err := budget.Call2(ctx, "home catalog discovery", h.Store.List)
	if err != nil {
		return nil, port.Incomplete
	}
	return ids, f
}

func (h Homes) checkClaims(ctx context.Context, s port.HomeScope, g port.HomeGeneration) port.Outcome {
	f, err := budget.Call(ctx, "home claim inspection", func(call context.Context) port.Outcome { return h.Store.CheckClaims(call, s, g) })
	if err != nil {
		return port.Incomplete
	}
	return f
}

type finiteHomeTransaction struct {
	port.HomeTransaction
	ctx  context.Context
	gate chan struct{}
}

func (t *finiteHomeTransaction) Read() (port.HomeScope, port.Outcome) {
	type result struct {
		s port.HomeScope
		f port.Outcome
	}
	r, err := held(t.ctx, t.gate, "home ownership read", func() result { s, f := t.HomeTransaction.Read(); return result{s, f} })
	if err != nil {
		return port.HomeScope{}, port.HomeUnavailable
	}
	return r.s, r.f
}
func (t *finiteHomeTransaction) Save(s port.HomeScope) port.Outcome {
	return sessionCall(t.ctx, t.gate, "home ownership publication", func() port.Outcome { return t.HomeTransaction.Save(s) })
}
func (t *finiteHomeTransaction) Close() {
	select {
	case <-t.gate:
		t.HomeTransaction.Close()
		t.gate <- struct{}{}
	default:
		go func() { <-t.gate; t.HomeTransaction.Close(); t.gate <- struct{}{} }()
	}
}

func (c finiteConnection) homeCall(ctx context.Context, name string, fn func(context.Context, port.HomeConnection) port.Outcome) port.Outcome {
	h, ok := c.RunConnection.(port.HomeConnection)
	if !ok {
		return port.HomeUnavailable
	}
	f, err := connectionCall(ctx, c.gate, name, func(call context.Context) port.Outcome { return fn(call, h) })
	if err != nil {
		return port.HomeUnavailable
	}
	return f
}
func (c finiteConnection) AllocateHome(ctx context.Context, h port.HomeReference) port.Outcome {
	return c.homeCall(ctx, "home allocation", func(call context.Context, hc port.HomeConnection) port.Outcome { return hc.AllocateHome(call, h) })
}
func (c finiteConnection) InspectHome(ctx context.Context, name string) (port.HomeObservation, port.Inspection) {
	h, ok := c.RunConnection.(port.HomeConnection)
	if !ok {
		return port.HomeObservation{}, port.Unavailable
	}
	type result struct {
		o      port.HomeObservation
		status port.Inspection
	}
	r, err := connectionCall(ctx, c.gate, "home inspection", func(call context.Context) result { o, s := h.InspectHome(call, name); return result{o, s} })
	if err != nil {
		return port.HomeObservation{}, port.Unavailable
	}
	return r.o, r.status
}
func (c finiteConnection) InitializeHome(ctx context.Context, id, uid, gid string) port.Outcome {
	return c.homeCall(ctx, "home initialization", func(call context.Context, h port.HomeConnection) port.Outcome {
		return h.InitializeHome(call, id, uid, gid)
	})
}
func (c finiteConnection) VerifyHomeRoot(ctx context.Context, id, uid, gid string) port.Outcome {
	return c.homeCall(ctx, "home root inspection", func(call context.Context, h port.HomeConnection) port.Outcome {
		return h.VerifyHomeRoot(call, id, uid, gid)
	})
}
func (c finiteConnection) RemoveHome(ctx context.Context, name string) port.Outcome {
	return c.homeCall(ctx, "home removal", func(call context.Context, h port.HomeConnection) port.Outcome { return h.RemoveHome(call, name) })
}
