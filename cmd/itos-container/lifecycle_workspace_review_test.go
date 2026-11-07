package main

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type lifecycleWorkspaceFault struct {
	sf.Workspace
	checks, blockAt, closes     int
	entered, release            chan struct{}
	pending, closedWhilePending bool
}

func (w *lifecycleWorkspaceFault) Unchanged() bool {
	w.checks++
	if w.checks == w.blockAt {
		w.pending = true
		close(w.entered)
		<-w.release
		w.pending = false
	}
	return w.Workspace.Unchanged()
}
func (w *lifecycleWorkspaceFault) Close() {
	w.closes++
	w.closedWhilePending = w.closedWhilePending || w.pending
	w.Workspace.Close()
}

func lifecycleWorkspaceRevalidationCase(t *testing.T, cause, phase, backend string) {
	l := lifecycleLocalFixture(t, backend)
	w := &lifecycleWorkspaceFault{blockAt: 1, entered: make(chan struct{}), release: make(chan struct{})}
	if phase == "start" {
		w.blockAt = 2
	}
	l.workspace = w
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
	<-w.entered
	wantCode, wantRule, deadline := 75, "run.call-timeout", 10*time.Second
	switch cause {
	case "startup timeout":
		wantRule, deadline = "run.startup-timeout", 120*time.Second
	case "SIGINT", "SIGTERM":
		cancel(sp.Interrupt{Signal: cause})
		wantCode, wantRule, deadline = 130, "run.cancelled", 0
		if cause == "SIGTERM" {
			wantCode = 143
		}
	}
	var got result
	returned := false
	select {
	case got = <-done:
		returned = true
	case <-time.After(deadline + 31*time.Second - time.Since(l.start)):
		t.Error("workspace revalidation exceeded its original operation and teardown budgets")
	}
	if returned {
		if got.code != wantCode || !strings.Contains(got.diagnostic, wantRule) || strings.Contains(got.diagnostic, "run.ownership") || time.Since(l.start) < deadline || time.Since(l.start) > deadline+30*time.Second {
			t.Errorf("revalidation result = %+v after %s; want %d %s within %s", got, time.Since(l.start), wantCode, wantRule, deadline+30*time.Second)
		}
		if cause == "call timeout" && !strings.Contains(got.diagnostic, "workspace revalidation") {
			t.Error("call timeout lost its safe operation name", got.diagnostic)
		}
		if strings.HasPrefix(cause, "SIG") && (!strings.Contains(got.diagnostic, cause) || !strings.Contains(got.diagnostic, "workload outcome not-started")) {
			t.Error("revalidation cancellation lost its first signal or known unstarted outcome", got.diagnostic)
		}
		synctest.Wait()
		if w.closes != 0 || w.closedWhilePending || !w.pending {
			t.Error("workspace pin was released before the pending check finished")
		}
	}
	creates := l.count("create")
	close(w.release)
	if !returned {
		<-done
	}
	synctest.Wait()
	wantCreates := 0
	if phase == "start" {
		wantCreates = 1
	}
	if w.closes != 1 || w.closedWhilePending || w.checks != w.blockAt || creates != wantCreates || l.count("create") != creates || l.count("start") != 0 || l.count("SIGTERM") != 0 || l.count("SIGINT") != 0 || l.count("SIGKILL") != 0 || len(l.m.e.Resources) != 1 {
		t.Error("late revalidation leaked authority, resumed launch or lost owned cleanup", w, l.events)
	}
	// These controls execute the same public command through the same workspace
	// port. A completed mismatch must still refuse; an unchanged pin must run.
	for _, changed := range []bool{false, true} {
		control := lifecycleLocalFixture(t, backend)
		control.workspace = &sf.Workspace{Changed: changed}
		code, diagnostic, _ := control.invoke(context.Background())
		if changed {
			if code != 1 || !strings.Contains(diagnostic, "run.ownership") || control.count("create") != 0 || control.count("start") != 0 {
				t.Error("confirmed workspace substitution did not refuse launch", code, diagnostic, control.events)
			}
		} else if code != 0 || diagnostic != "" || control.count("create") != 1 || control.count("start") != 1 || control.count("remove") != 1 {
			t.Error("healthy workspace control failed", code, diagnostic, control.events)
		}
		if len(control.m.e.Resources) != 1 {
			t.Error("workspace control changed unrelated resources")
		}
	}
}
