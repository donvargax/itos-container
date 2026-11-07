package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

type lifecycleWaitEngine struct {
	sp.RunEngine
	opened, waits, closes       int
	entered, release            chan struct{}
	pending, closedWhilePending bool
	healthy                     bool
}

func (e *lifecycleWaitEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	c, f := e.RunEngine.OpenRun(ctx, b)
	if c == nil {
		return nil, f
	}
	e.opened++
	return lifecycleWaitConnection{c, e, e.opened == 1}, f
}

type lifecycleWaitConnection struct {
	sp.RunConnection
	e      *lifecycleWaitEngine
	launch bool
}

func (c lifecycleWaitConnection) Wait(ctx context.Context, id string) (int, sp.Outcome) {
	c.e.waits++
	if c.launch {
		if c.e.healthy {
			// An acknowledged foreground subscription outlives both finite calls
			// and startup. A wrapper must not impose either lifetime on it.
			time.Sleep(121 * time.Second)
			return c.RunConnection.Wait(ctx, id)
		}
		// The worker exits, but the launch connection loses its response. Fresh
		// inspection can see exited without supplying an authoritative exit code.
		_, _ = c.RunConnection.Wait(ctx, id)
		return 0, sp.EngineLost
	}
	c.e.pending = true
	close(c.e.entered)
	<-c.e.release // Like a socket stat, this cannot observe context cancellation.
	c.e.pending = false
	return 17, ""
}

func (c lifecycleWaitConnection) Close() {
	if !c.launch {
		c.e.closes++
		c.e.closedWhilePending = c.e.closedWhilePending || c.e.pending
	}
	c.RunConnection.Close()
}

func lifecycleTeardownWaitCase(t *testing.T, backend string) {
	l := lifecycleLocalFixture(t, backend)
	f := &lifecycleWaitEngine{RunEngine: lifecycleEngine{l}, entered: make(chan struct{}), release: make(chan struct{})}
	released := false
	defer func() {
		if !released {
			close(f.release)
		}
	}()
	l.engine = f
	type result struct {
		code       int
		diagnostic string
	}
	done := make(chan result, 1)
	go func() {
		code, diagnostic, _ := l.invoke(context.Background())
		done <- result{code, diagnostic}
	}()
	// Race both barriers against a guard so the broken synchronous wait fails an
	// assertion and can be released without hanging the test suite.
	guard := time.After(31 * time.Second)
	select {
	case <-f.entered:
	case <-done:
		t.Fatal("run returned before reaching the recovery wait")
	case <-guard:
		t.Fatal("run did not reach the recovery wait")
	}
	var got result
	returned := false
	select {
	case got = <-done:
		returned = true
	case <-guard:
		t.Error("controller still waits for the cancellation-ignoring exit observation beyond its teardown deadline")
	}
	id := strings.Repeat("a", 64)
	var before sp.PublicRecord
	if returned {
		if got.code != 75 || time.Since(l.start) > 30*time.Second || !strings.Contains(got.diagnostic, "run.engine-lost") || !strings.Contains(got.diagnostic, "run.teardown-timeout") || !strings.Contains(got.diagnostic, "workload outcome unknown") || !strings.Contains(got.diagnostic, id) {
			t.Errorf("teardown result = %+v after %s", got, time.Since(l.start))
		}
		synctest.Wait()
		if f.closes != 0 || f.closedWhilePending || !f.pending || f.waits != 2 {
			t.Error("pending exit observation lost its connection", f)
		}
		entry, failure := l.registry.Open(context.Background(), id, true)
		if failure != "" {
			t.Fatal("lost recovery record", failure)
		}
		before = entry.Record()
		if before.State == "stopped" || before.WorkloadExit != nil || entry.Intent().Record().Stage == "absent" {
			t.Error("pending exit observation fabricated completion", before)
		}
		entry.Close()
	}
	close(f.release)
	released = true
	if !returned {
		<-done
	}
	synctest.Wait()
	if f.closes != 1 || f.closedWhilePending {
		t.Error("late exit observation did not release its retained connection exactly once", f.closes, f.closedWhilePending)
	}
	if returned {
		entry, failure := l.registry.Open(context.Background(), id, true)
		if failure != "" {
			t.Fatal("lost recovery after late observation", failure)
		}
		if !reflect.DeepEqual(before, entry.Record()) {
			t.Error("late exit observation changed the returned recovery record", entry.Record())
		}
		entry.Close()
	}
	if l.count("remove") != 0 || len(l.m.e.Resources) != 2 {
		t.Error("abandoned observation resumed cleanup or lost owned state", l.events)
	}
	recovered := (domain.Recovery{Registry: l.registry, Engine: lifecycleEngine{l}}).Stop(context.Background(), id)
	if len(recovered.Failures) != 0 || len(recovered.Entries) != 1 || recovered.Entries[0].Cleanup != "complete" || len(l.m.e.Resources) != 1 || l.count("create") != 1 || l.count("start") != 1 {
		t.Error("fresh recovery failed to clean only the original worker", recovered, l.events)
	}
	control := lifecycleLocalFixture(t, backend)
	control.m.e.Exit = 17
	control.engine = &lifecycleWaitEngine{RunEngine: lifecycleEngine{control}, healthy: true}
	code, diagnostic, _ := control.invoke(context.Background())
	if code != 17 || diagnostic != "" || time.Since(control.start) != 121*time.Second || control.count("create") != 1 || control.count("start") != 1 || control.count("remove") != 1 || len(control.m.e.Resources) != 1 {
		t.Error("long healthy foreground wait was truncated or lost its exit", code, diagnostic, control.events)
	}
}
