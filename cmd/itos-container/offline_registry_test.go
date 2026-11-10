package main

import (
	"context"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

// Observe the registered lifecycle without replacing its state or outcomes.
// Registry.Open, rather than the old Store.OpenCleanup, now owns the transition
// boundary at which the unclassified adapter failure is injected.
type offlineRegistry struct {
	sp.Registry
	o *offlineObserver
}

func (r offlineRegistry) ForRun(d ap.Durable) sp.Registry {
	if bound, ok := r.Registry.(sp.RunRegistry); ok {
		r.Registry = bound.ForRun(d)
	}
	return r
}

func (r offlineRegistry) Open(ctx context.Context, id string, readOnly bool) (sp.Session, sp.Outcome) {
	if r.o.exited && r.o.drained != nil {
		// This cleanup-cause fixture uses a healthy output stream. Hold its
		// registry call until the actual drain is recorded, selecting the
		// drain-before-reopen schedule asserted by its positive control.
		// Registered run drains concurrently; scheduler luck must not decide
		// that order or race the observer's record/durability snapshots.
		// A missing drain still fails within the original operation budget.
		select {
		case <-r.o.drained:
		case <-ctx.Done():
			return nil, sp.Incomplete
		}
	}
	if r.o.exited && r.o.request.CleanupCause == "internal" {
		f := sp.Outcome("slice-13-private-adapter-failure")
		r.o.event(offlineEvent{Operation: "reopen", Failure: string(f)})
		return nil, f
	}
	s, f := r.Registry.Open(ctx, id, readOnly)
	if f == "" {
		r.o.record, r.o.binding = s.Intent().Record(), s.Intent().Binding()
	}
	r.o.event(offlineEvent{Operation: "reopen", Failure: string(f)})
	if f != "" {
		return s, f
	}
	return offlineSession{Session: s, o: r.o}, ""
}

type offlineSession struct {
	sp.Session
	o *offlineObserver
}

func (s offlineSession) Intent() sp.Intent {
	return offlineIntent{Intent: s.Session.Intent(), o: s.o, launch: !s.o.exited}
}

func (s offlineSession) Validate() sp.Outcome {
	f := s.Session.Validate()
	s.o.event(offlineEvent{Operation: "validate", Failure: string(f)})
	return f
}

type offlineRunEntropy struct {
	public, token sp.Entropy
	drawn         bool
}

func (e *offlineRunEntropy) Token() (string, sp.Outcome) {
	if !e.drawn {
		e.drawn = true
		return e.public.Token()
	}
	return e.token.Token()
}
