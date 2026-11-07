package main

import (
	"context"
	"strings"
	"testing"
	"time"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	bp "github.com/donvargax/itos-container/internal/engine/budget/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

// Lifetime is copied after the call's cancellation precheck and before its
// goroutine is dispatched. Observe that lookup through a test-only context to
// cancel in this narrow window without depending on goroutine scheduling.
type lifecycleDispatchContext struct {
	context.Context
	key    any
	armed  bool
	fired  bool
	cancel context.CancelCauseFunc
	signal string
}

func (c *lifecycleDispatchContext) Value(key any) any {
	if c.key == nil {
		c.key = key
	} else if c.armed && key == c.key {
		c.armed, c.fired = false, true
		c.cancel(sp.Interrupt{Signal: c.signal})
	}
	return c.Context.Value(key)
}

type lifecycleDispatchRegistry struct {
	sp.Registry
	ctx *lifecycleDispatchContext
}

func (r lifecycleDispatchRegistry) ForRun(d ap.Durable) sp.Registry {
	if bound, ok := r.Registry.(sp.RunRegistry); ok {
		r.Registry = bound.ForRun(d)
	}
	return r
}
func (r lifecycleDispatchRegistry) Open(ctx context.Context, id string, ro bool) (sp.Session, sp.Outcome) {
	s, f := r.Registry.Open(ctx, id, ro)
	if s != nil && !r.ctx.fired {
		s = lifecycleDispatchSession{s, r.ctx}
	}
	return s, f
}

type lifecycleDispatchSession struct {
	sp.Session
	ctx *lifecycleDispatchContext
}

func (s lifecycleDispatchSession) Intent() sp.Intent {
	return lifecycleDispatchIntent{s.Session.Intent(), s.ctx}
}

type lifecycleDispatchIntent struct {
	sp.Intent
	ctx *lifecycleDispatchContext
}

func (i lifecycleDispatchIntent) Binding() sp.Binding {
	b := i.Intent.Binding()
	// Loading the acquired session's binding is the last preparation before
	// workspace pinning. The next finite call is the one to interrupt.
	i.ctx.armed = true
	return b
}

type lifecycleDispatchWorkspace struct {
	sf.Workspace
	pins, closes int
}

func (w *lifecycleDispatchWorkspace) Pin(b sp.Binding) sp.Outcome {
	w.pins++
	return w.Workspace.Pin(b)
}
func (w *lifecycleDispatchWorkspace) Close() { w.closes++; w.Workspace.Close() }

func lifecycleWorkspaceDispatchCase(t *testing.T, signal, backend string) {
	want := 130
	if signal == "SIGTERM" {
		want = 143
	}
	// Cancellation and the gate are both ready before dispatch. Repetition
	// exercises the select's two choices; every choice must leave the pin alone.
	for range 64 {
		l := lifecycleLocalFixture(t, backend)
		parent, cancel := context.WithCancelCause(context.Background())
		ctx := &lifecycleDispatchContext{Context: parent, cancel: cancel, signal: signal}
		bp.Lifetime(ctx) // Capture the opaque lifetime key through its public API.
		l.registry = lifecycleDispatchRegistry{l.registry, ctx}
		w := &lifecycleDispatchWorkspace{Workspace: *sf.MountWorkspace(l.m.b)}
		l.workspace = w
		code, diagnostic, _ := l.invoke(ctx) // Also joins dispatched calls before assertions.
		cancel(nil)
		if !ctx.fired || code != want || !strings.Contains(diagnostic, "run.cancelled") || !strings.Contains(diagnostic, signal) || !strings.Contains(diagnostic, "workload outcome not-started") || time.Since(l.start) > 30*time.Second {
			t.Fatal("dispatch interruption lost its bounded cancellation result", ctx.fired, code, diagnostic)
		}
		if w.pins != 0 || w.closes != 1 || l.count("create") != 0 || l.count("start") != 0 || len(l.m.e.Resources) != 1 {
			t.Fatal("cancelled dispatch opened a workspace pin or launched work", w.pins, w.closes, l.events)
		}
	}
	control := lifecycleLocalFixture(t, backend)
	w := &lifecycleDispatchWorkspace{Workspace: *sf.MountWorkspace(control.m.b)}
	control.workspace = w
	code, diagnostic, _ := control.invoke(context.Background())
	if code != 0 || diagnostic != "" || w.pins != 1 || w.closes != 1 || !w.Closed || control.count("create") != 1 || control.count("start") != 1 || control.count("remove") != 1 || len(control.m.e.Resources) != 1 {
		t.Fatal("healthy workspace acquisition failed", code, diagnostic, w, control.events)
	}
}
