package domain

import (
	"context"
	"errors"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Finite request bounds are part of the engine contract, including fake-backed
// controllers. Returned storage/connection handles retain the phase lifetime.
type finiteRegistry struct{ port.Registry }

func (r finiteRegistry) ForRun(d approval.Durable) port.Registry {
	if bound, ok := r.Registry.(port.RunRegistry); ok {
		return finiteRegistry{bound.ForRun(d)}
	}
	return r
}
func (r finiteRegistry) List(ctx context.Context) ([]string, port.Outcome) {
	ids, f, err := budget.Call2(ctx, "session discovery", func(call context.Context) ([]string, port.Outcome) { return r.Registry.List(call) })
	if err != nil {
		return nil, port.Incomplete
	}
	return ids, f
}
func (r finiteRegistry) Open(ctx context.Context, id string, ro bool) (port.Session, port.Outcome) {
	type result struct {
		s port.Session
		f port.Outcome
	}
	var v result
	for {
		var err error
		v, err = budget.Acquire(ctx, "session lock acquisition", func(call context.Context) result { s, f := r.Registry.Open(call, id, ro); return result{s, f} }, func(v result) {
			if v.s != nil {
				v.s.Close()
			}
		})
		if err == nil {
			break
		}
		var timeout budget.Failure
		// Retrying a lock observation is safe; preserve stop's total and never
		// retry a mutation. A startup scope aborts on its first exhausted call.
		if budget.Cause(ctx) != nil || !errors.As(err, &timeout) {
			return nil, port.Incomplete
		}
	}
	if v.s == nil {
		return nil, v.f
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	s := &finiteSession{Session: v.s, ctx: ctx, gate: gate, record: v.s.Record()}
	i := v.s.Intent()
	s.intent = &finiteIntent{Intent: i, ctx: ctx, gate: gate, record: i.Record(), binding: i.Binding()}
	return s, v.f
}

type finiteSession struct {
	port.Session
	ctx    context.Context
	gate   chan struct{}
	record port.PublicRecord
	intent *finiteIntent
}

func (s *finiteSession) Record() port.PublicRecord { return s.record }
func (s *finiteSession) Intent() port.Intent       { return s.intent }
func (s *finiteSession) Close() {
	// A timed-out publication can still finish. Keep its lock held until that
	// operation returns; a new controller gets its own bounded lock attempt.
	select {
	case <-s.gate:
		s.Session.Close()
		s.gate <- struct{}{}
	default:
		go func() { <-s.gate; s.Session.Close(); s.gate <- struct{}{} }()
	}
}
func held[T any](ctx context.Context, gate chan struct{}, name string, fn func() T) (T, error) {
	return budget.Call(ctx, name, func(call context.Context) T {
		select {
		case <-gate:
			defer func() { gate <- struct{}{} }()
			return fn()
		case <-call.Done():
			var zero T
			return zero
		}
	})
}
func sessionCall(ctx context.Context, gate chan struct{}, name string, fn func() port.Outcome) port.Outcome {
	f, err := held(ctx, gate, name, fn)
	if err != nil {
		return port.Incomplete
	}
	return f
}
func (s *finiteSession) Validate() port.Outcome {
	return sessionCall(s.ctx, s.gate, "session validation", s.Session.Validate)
}
func (s *finiteSession) ValidateLaunch() port.Outcome {
	return sessionCall(s.ctx, s.gate, "launch validation", s.Session.ValidateLaunch)
}
func (s *finiteSession) Sync() port.Outcome {
	return sessionCall(s.ctx, s.gate, "completion publication", s.Session.Sync)
}
func (s *finiteSession) Save(r port.PublicRecord) port.Outcome {
	type result struct {
		failure port.Outcome
		record  port.PublicRecord
	}
	v, err := held(s.ctx, s.gate, "session publication", func() result { f := s.Session.Save(r); return result{f, s.Session.Record()} })
	if err != nil {
		return port.Incomplete
	}
	// A failed fsync may leave a complete visible record. Preserve the adapter's
	// observed representation without claiming durability or rewriting a tombstone.
	s.record = v.record
	return v.failure
}

func (s *finiteSession) Controller() (string, port.Outcome) {
	type result struct {
		state string
		f     port.Outcome
	}
	r, err := held(s.ctx, s.gate, "controller inspection", func() result { v, f := s.Session.Controller(); return result{v, f} })
	if err != nil {
		return "unknown", port.Incomplete
	}
	return r.state, r.f
}

type finiteIntent struct {
	port.Intent
	ctx     context.Context
	gate    chan struct{}
	record  port.Record
	binding port.Binding
}

func (i *finiteIntent) Record() port.Record   { return i.record }
func (i *finiteIntent) Binding() port.Binding { return i.binding }
func (i *finiteIntent) Validate() port.Outcome {
	return sessionCall(i.ctx, i.gate, "intent validation", i.Intent.Validate)
}
func (i *finiteIntent) Save(r port.Record) port.Outcome {
	type result struct {
		failure port.Outcome
		record  port.Record
	}
	v, err := held(i.ctx, i.gate, "intent publication", func() result { f := i.Intent.Save(r); return result{f, i.Intent.Record()} })
	if err != nil {
		return port.Incomplete
	}
	i.record = v.record
	return v.failure
}

func (*finiteIntent) Close() {}

type finiteEngine struct{ port.RunEngine }

func (e finiteEngine) OpenRun(ctx context.Context, b port.Binding) (port.RunConnection, port.Outcome) {
	type result struct {
		c port.RunConnection
		f port.Outcome
	}
	r, err := budget.Acquire(ctx, "engine connection", func(call context.Context) result { c, f := e.RunEngine.OpenRun(call, b); return result{c, f} }, func(r result) {
		if r.c != nil {
			r.c.Close()
		}
	})
	if err != nil {
		return nil, port.Incomplete
	}
	if r.c == nil {
		return nil, r.f
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return finiteConnection{r.c, gate}, r.f
}

type finiteConnection struct {
	port.RunConnection
	gate chan struct{}
}

type finiteWorkspace struct {
	port.Workspace
	ctx  context.Context
	gate chan struct{}
}

func (w finiteWorkspace) Pin(b port.Binding) port.Outcome {
	f := sessionCall(w.ctx, w.gate, "workspace pin", func() port.Outcome { return w.Workspace.Pin(b) })
	if w.ctx.Err() != nil {
		w.Close()
	}
	return f
}
func (w finiteWorkspace) Validate() port.Outcome {
	f, err := held(w.ctx, w.gate, "workspace revalidation", w.Workspace.Validate)
	if err != nil {
		if state, ok := w.ctx.Value(startupKey{}).(*startupState); ok {
			state.operation = "workspace revalidation"
		}
		return port.Incomplete
	}
	return f
}
func (w finiteWorkspace) Close() {
	select {
	case <-w.gate:
		w.Workspace.Close()
		w.gate <- struct{}{}
	default:
		go func() { <-w.gate; w.Workspace.Close(); w.gate <- struct{}{} }()
	}
}

func (c finiteConnection) Check(ctx context.Context) port.Outcome {
	f, err := connectionCall(ctx, c.gate, "engine inspection", c.RunConnection.Check)
	if err != nil {
		return port.Incomplete
	}
	return f
}
func (c finiteConnection) Inspect(ctx context.Context, id string) (port.Observation, port.Inspection) {
	type result struct {
		o port.Observation
		f port.Inspection
	}
	r, err := connectionCall(ctx, c.gate, "resource inspection", func(call context.Context) result {
		o, f := c.RunConnection.Inspect(call, id)
		return result{o, f}
	})
	if err != nil {
		return port.Observation{}, port.Unavailable
	}
	return r.o, r.f
}
func (c finiteConnection) Signal(ctx context.Context, id, signal string) port.Outcome {
	f, err := connectionCall(ctx, c.gate, "signal delivery", func(call context.Context) port.Outcome { return c.RunConnection.Signal(call, id, signal) })
	if err != nil {
		return port.Incomplete
	}
	return f
}
func (c finiteConnection) Remove(ctx context.Context, id string) {
	_, _ = connectionCall(ctx, c.gate, "resource removal", func(call context.Context) bool { c.RunConnection.Remove(call, id); return true })
}
