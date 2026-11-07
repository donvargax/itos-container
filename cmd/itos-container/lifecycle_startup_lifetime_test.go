package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

// The barrier models a socket stat that cannot observe its request context.
// Create has already been accepted; attach has already acquired a handle. Start
// is held before its request, so cancellation must prevent that late mutation.
type lifecycleLaunchFault struct {
	// The port fake shares its engine journal across connections. Synchronize
	// completed fake calls with fresh recovery opens while a reply is withheld.
	mu                          sync.Mutex
	operation                   string
	entered, release            chan struct{}
	pending, closedWhilePending bool
	closes, attachmentCloses    int
}

func (f *lifecycleLaunchFault) block(operation string) {
	if f.operation != operation {
		return
	}
	f.pending = true
	close(f.entered)
	<-f.release
	f.pending = false
}

type lifecycleLaunchEngine struct {
	sp.RunEngine
	f      *lifecycleLaunchFault
	opened bool
}

func (e *lifecycleLaunchEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	c, f := e.RunEngine.OpenRun(ctx, b)
	if c == nil || e.opened {
		return c, f
	}
	e.opened = true
	return lifecycleLaunchConnection{c, e.f}, f
}

type lifecycleLaunchConnection struct {
	sp.RunConnection
	f *lifecycleLaunchFault
}

func (c lifecycleLaunchConnection) CreateRun(ctx context.Context, r sp.Record, argv []string) (string, sp.Outcome) {
	c.f.mu.Lock()
	id, f := c.RunConnection.CreateRun(ctx, r, argv)
	c.f.mu.Unlock()
	c.f.block("create response")
	return id, f
}
func (c lifecycleLaunchConnection) VerifyRun(ctx context.Context, id string, argv []string) sp.Outcome {
	c.f.mu.Lock()
	f := c.RunConnection.VerifyRun(ctx, id, argv)
	c.f.mu.Unlock()
	c.f.block("boundary inspection")
	return f
}
func (c lifecycleLaunchConnection) Attach(ctx context.Context, id string, streams sp.Streams) (sp.Attachment, sp.Outcome) {
	c.f.mu.Lock()
	a, f := c.RunConnection.Attach(ctx, id, streams)
	c.f.mu.Unlock()
	c.f.block("attach establishment")
	if a == nil {
		return nil, f
	}
	return lifecycleLaunchAttachment{a, c.f}, f
}
func (c lifecycleLaunchConnection) Start(ctx context.Context, id string) sp.Outcome {
	c.f.block("start acknowledgement")
	if ctx.Err() != nil {
		return sp.ExecutionFailed
	}
	return c.RunConnection.Start(ctx, id)
}
func (c lifecycleLaunchConnection) Close() {
	c.f.closes++
	c.f.closedWhilePending = c.f.closedWhilePending || c.f.pending
	c.RunConnection.Close()
}

type lifecycleLaunchAttachment struct {
	sp.Attachment
	f *lifecycleLaunchFault
}

func (a lifecycleLaunchAttachment) Close() {
	a.f.attachmentCloses++
	a.Attachment.Close()
}

func lifecycleStartupLifetimeCase(t *testing.T, cause, operation, backend string) {
	l := lifecycleLocalFixture(t, backend)
	f := &lifecycleLaunchFault{operation: operation, entered: make(chan struct{}), release: make(chan struct{})}
	l.engine = &lifecycleLaunchEngine{RunEngine: lifecycleEngine{l}, f: f}
	preflight := &lifecyclePreflightFault{}
	if cause == "startup timeout" {
		preflight.progress = 115 * time.Second
	}
	l.approval, l.snapshots = lifecycleBudgetPreflight(l, preflight)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	type result struct {
		code       int
		diagnostic string
	}
	done := make(chan result, 1)
	go func() {
		code, diagnostic, _ := l.invoke(ctx)
		done <- result{code, diagnostic}
	}()
	<-f.entered
	wantCode, wantRule, deadline := 75, "run.call-timeout", 10*time.Second
	if cause == "startup timeout" {
		wantRule, deadline = "run.startup-timeout", 120*time.Second
	}
	if cause == "SIGINT" {
		cancel(sp.Interrupt{Signal: cause})
		wantCode, wantRule, deadline = 130, "run.cancelled", 0
	}
	// This guard releases a broken synchronous implementation and records an
	// assertion failure instead of deadlocking the virtual-time test bubble.
	var got result
	returned := false
	select {
	case got = <-done:
		returned = true
	case <-time.After(deadline + 30*time.Second - time.Since(l.start) + time.Second):
		t.Error("controller still waits for the cancellation-ignoring launch operation beyond its deadline and teardown budget")
	}
	if returned {
		if got.code != wantCode || !strings.Contains(got.diagnostic, wantRule) || time.Since(l.start) < deadline || time.Since(l.start) > deadline+30*time.Second {
			t.Errorf("interruption result = %+v after %s; want %d %s within %s", got, time.Since(l.start), wantCode, wantRule, deadline+30*time.Second)
		}
		synctest.Wait()
		if f.closes != 0 || f.closedWhilePending || !f.pending {
			t.Error("pending launch operation lost its connection before returning")
		}
	}
	creates, starts := l.count("create"), l.count("start")
	close(f.release)
	if !returned {
		<-done
	}
	synctest.Wait()
	if f.closes != 1 || f.closedWhilePending {
		t.Error("late launch operation did not release its retained connection exactly once", f.closes, f.closedWhilePending)
	}
	if (operation == "attach establishment" || operation == "start acknowledgement") && f.attachmentCloses != 1 {
		t.Error("late or established attachment was not closed exactly once", f.attachmentCloses)
	}
	if l.count("create") != creates || l.count("start") != starts || starts != 0 {
		t.Error("late completion resumed launch or retried a mutation", l.events)
	}
	if len(l.m.e.Resources) != 1 {
		t.Error("cleanup lost an accepted mutation or changed unrelated state", l.m.e.Resources)
	}
	control := lifecycleLocalFixture(t, backend)
	healthy := &lifecycleLaunchFault{}
	control.engine = &lifecycleLaunchEngine{RunEngine: lifecycleEngine{control}, f: healthy}
	control.approval, control.snapshots = lifecycleBudgetPreflight(control, &lifecyclePreflightFault{})
	code, diagnostic, _ := control.invoke(context.Background())
	if code != 0 || diagnostic != "" || control.count("create") != 1 || control.count("start") != 1 || control.count("remove") != 1 || len(control.m.e.Resources) != 1 || healthy.closes != 1 || healthy.attachmentCloses != 1 {
		t.Error("healthy launch control failed", code, diagnostic, control.events)
	}
}
