package domain

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func (s Preflight) InspectContext(ctx context.Context) port.View {
	s.Host = boundedHost{s.Host, ctx}
	s.Store = boundedStore{s.Store, ctx}
	s.Local = boundedReadiness{s.Local, ctx}
	return s.Inspect()
}
func (s Preflight) FreezeContext(ctx context.Context, v port.View) (port.Frozen, port.Failure) {
	s.Host = boundedHost{s.Host, ctx}
	s.Store = boundedStore{s.Store, ctx}
	return s.Freeze(v)
}

type boundedHost struct {
	port.Host
	ctx context.Context
}

func (h boundedHost) Identity() (string, string) {
	a, b, _ := budget.Call2(h.ctx, "host identity", func(context.Context) (string, string) { return h.Host.Identity() })
	return a, b
}
func (h boundedHost) Socket(endpoint string) string {
	v, _ := budget.Call(h.ctx, "socket identity", func(context.Context) string { return h.Host.Socket(endpoint) })
	return v
}

type boundedStore struct {
	port.Store
	ctx context.Context
}

func (s boundedStore) Read(c port.Candidate) (port.Record, port.Failure) {
	r, f, err := budget.Call2(s.ctx, "preflight approval state read", func(context.Context) (port.Record, port.Failure) { return s.Store.Read(c) })
	if err != nil {
		return port.Record{}, "state.unavailable"
	}
	return r, f
}
func (s boundedStore) Select(c port.Candidate, g string) (port.Frozen, port.Failure) {
	r, f, err := budget.Call2(s.ctx, "approval selection", func(context.Context) (port.Frozen, port.Failure) { return s.Store.Select(c, g) })
	if err != nil {
		return port.Frozen{}, "state.unavailable"
	}
	return r, f
}

type boundedReadiness struct {
	port.StagedReadiness
	ctx context.Context
}

func (s boundedReadiness) InspectBeforeEngine(check func(rp.Result) bool) rp.Result {
	if contextual, ok := s.StagedReadiness.(port.ContextReadiness); ok {
		return contextual.InspectBeforeEngineContext(s.ctx, check)
	}
	r, _ := budget.Call(s.ctx, "preflight engine inspection", func(context.Context) rp.Result { return s.StagedReadiness.InspectBeforeEngine(check) })
	return r
}
