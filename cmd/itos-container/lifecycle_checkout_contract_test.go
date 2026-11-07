package main

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func lifecycleCheckoutLifetimeCase(t *testing.T, cause, operation, backend string) {
	l := lifecycleLocalFixture(t, backend)
	entered, release := make(chan struct{}), make(chan struct{})
	f := &lifecyclePreflightFault{}
	f.block = func(name string) {
		if name == operation {
			f.pending = true
			close(entered)
			<-release
			f.pending = false
		}
	}
	l.approval, l.snapshots = lifecycleBudgetPreflight(l, f)
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
	<-entered
	wantCode, wantRule, wantElapsed := 75, "run.call-timeout", 10*time.Second
	if cause == "SIGINT" {
		cancel(sp.Interrupt{Signal: cause})
		wantCode, wantRule, wantElapsed = 130, "run.cancelled", 0
	}
	got := <-done
	if got.code != wantCode || !strings.Contains(got.diagnostic, wantRule) || time.Since(l.start) != wantElapsed {
		t.Errorf("interruption result = %+v after %s; want %d %s after %s", got, time.Since(l.start), wantCode, wantRule, wantElapsed)
	}
	synctest.Wait()
	if f.closedWhilePending || f.closes != 0 {
		t.Error("checkout closed while its operation was still pending")
	}
	steps := len(f.events)
	close(release)
	synctest.Wait()
	if f.closedWhilePending || f.closes != 1 || f.checkout.Opened {
		t.Errorf("late checkout release: closed while pending %v, closes %d, still pinned %v", f.closedWhilePending, f.closes, f.checkout.Opened)
	}
	if len(f.events) != steps || len(l.events) != 0 {
		t.Error("late completion entered a later phase or launched a worker", f.events, l.events)
	}
	// The complete command succeeds on the same path when only the stall clears.
	control := lifecycleLocalFixture(t, backend)
	control.approval, control.snapshots = lifecycleBudgetPreflight(control, &lifecyclePreflightFault{})
	code, diagnostic, _ := control.invoke(context.Background())
	if code != 0 || diagnostic != "" || control.count("create") != 1 || control.count("start") != 1 || control.count("remove") != 1 || len(control.m.e.Resources) != 1 {
		t.Error("healthy checkout control failed", code, diagnostic, control.events)
	}
}
