package domain

import (
	"context"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	mp "github.com/donvargax/itos-container/internal/engine/manifest/port"
	pp "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// InspectBeforeEngineContext preserves the aggregate phase while each external
// observation receives its own finite budget. Legacy doctor inspection keeps its
// existing port contract; run opts into these whole-invocation bounds.
func (i Inspector) InspectBeforeEngineContext(ctx context.Context, input port.Input, check func(port.Result) bool) port.Result {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	i.Checkout = boundedCheckout{i.Checkout, ctx, gate}
	i.Policy = boundedPolicy{i.Policy, ctx, gate}
	i.Files = boundedFiles{i.Files, ctx, gate}
	i.Engine = boundedEngine{i.Engine, ctx}
	return i.InspectBeforeEngine(input, check)
}

type boundedCheckout struct {
	port.Checkout
	ctx  context.Context
	gate chan struct{}
}
type boundedPolicy struct {
	pp.Source
	ctx  context.Context
	gate chan struct{}
}
type boundedFiles struct {
	mp.Files
	ctx  context.Context
	gate chan struct{}
}
type boundedEngine struct {
	port.Engine
	ctx context.Context
}
type boundedSession struct {
	port.Session
	ctx context.Context
}

func (b boundedCheckout) Pin(root string) port.Failure {
	v, err := checkoutCall(b.ctx, b.gate, "checkout pin", func() port.Failure { return b.Checkout.Pin(root) })
	if err != nil {
		// InspectBeforeEngine defers Close only after a successful Pin. A late
		// pin still owns resources and must release them after it finishes.
		b.Close()
		return port.CheckoutUnavailable
	}
	return v
}
func (b boundedCheckout) Unchanged() bool {
	v, err := checkoutCall(b.ctx, b.gate, "checkout revalidation", b.Checkout.Unchanged)
	if err != nil {
		return false
	}
	return v
}
func (b boundedSession) Unchanged() bool {
	v, err := budget.Call(b.ctx, "socket revalidation", func(context.Context) bool { return b.Session.Unchanged() })
	if err != nil {
		return false
	}
	return v
}
func (b boundedCheckout) Inspect() (port.CheckoutIdentity, port.Failure) {
	a, c, err := checkoutCall2(b.ctx, b.gate, "checkout inspection", b.Checkout.Inspect)
	if err != nil {
		return port.CheckoutIdentity{}, port.CheckoutUnavailable
	}
	return a, c
}
func (b boundedPolicy) Read() (pp.Value, []pp.Issue) {
	a, c, err := checkoutCall2(b.ctx, b.gate, "policy read", b.Source.Read)
	if err != nil {
		return pp.Value{}, nil
	}
	return a, c
}
func (b boundedFiles) Read(path string) ([]byte, mp.Failure) {
	a, c, err := checkoutCall2(b.ctx, b.gate, "manifest read", func() ([]byte, mp.Failure) { return b.Files.Read(path) })
	if err != nil {
		return nil, mp.Path
	}
	return a, c
}
func (b boundedSession) Version() (port.Version, port.Failure) {
	a, c, err := budget.Call2(b.ctx, "engine version", func(call context.Context) (port.Version, port.Failure) { return b.current(call).Version() })
	if err != nil {
		return port.Version{}, port.EngineTimeout
	}
	return a, c
}
func (b boundedSession) Info() (port.Info, port.Failure) {
	a, c, err := budget.Call2(b.ctx, "preflight engine inspection", func(call context.Context) (port.Info, port.Failure) { return b.current(call).Info() })
	if err != nil {
		return port.Info{}, port.EngineTimeout
	}
	return a, c
}
func (b boundedSession) Image(ref string) (port.Image, port.Failure) {
	a, c, err := budget.Call2(b.ctx, "image inspection", func(call context.Context) (port.Image, port.Failure) { return b.current(call).Image(ref) })
	if err != nil {
		return port.Image{}, port.EngineTimeout
	}
	return a, c
}

func (b boundedEngine) Open(selection, root string, _ time.Duration) (port.Session, port.Failure) {
	type result struct {
		session port.Session
		failure port.Failure
	}
	r, err := budget.Acquire(b.ctx, "engine connection", func(call context.Context) result {
		d, _ := call.Deadline()
		var s port.Session
		var f port.Failure
		if contextual, ok := b.Engine.(port.ContextEngine); ok {
			s, f = contextual.OpenContext(call, selection, root)
		} else {
			s, f = b.Engine.Open(selection, root, time.Until(d))
		}
		return result{s, f}
	}, func(r result) {
		if r.session != nil {
			r.session.Close()
		}
	})
	if err != nil {
		return nil, port.EngineTimeout
	}
	if r.session == nil {
		return nil, r.failure
	}
	return boundedSession{r.session, b.ctx}, r.failure
}

func (b boundedSession) current(ctx context.Context) port.Session {
	if contextual, ok := b.Session.(port.ContextSession); ok {
		return contextual.ForContext(ctx)
	}
	return b.Session
}
