package domain

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func runningRecoveryFixture(t *testing.T) (port.Session, port.RunConnection, *f.Registry, *f.RunEngine) {
	t.Helper()
	runner, registry, engine, _, id := registeredSessionFixture(t)
	c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
	if failure != "" || worker == "" {
		t.Fatal("launch fixture", worker, failure)
	}
	t.Cleanup(c.Close)
	t.Cleanup(a.Close)
	s, failure := registry.Open(t.Context(), id, false)
	if failure != "" {
		t.Fatal("recovery fixture", failure)
	}
	t.Cleanup(s.Close)
	engine.Exit = 17
	return s, c, registry, engine
}

func TestForegroundStopRetainsWorkerWhenObservedExitCannotBeSaved(t *testing.T) {
	s, c, registry, engine := runningRecoveryFixture(t)
	o := engine.Resources[engine.CreateID]
	o.Status, o.Running, o.PID = "exited", false, 0
	engine.Resources[engine.CreateID] = o
	registry.BeforeSave = func(r port.PublicRecord) {
		if r.WorkloadExit != nil {
			registry.SaveFailure = port.IntentWrite
		}
	}
	ctx := context.WithValue(t.Context(), runGraceKey{}, time.Now().Add(10*time.Second))
	failure := stopSession(ctx, s, c, "SIGTERM", nil, true)
	if failure != port.IntentWrite || s.Record().WorkloadExit != nil || s.Record().State != "stopping" {
		t.Fatal("failed exit publication claimed durable progress", failure, s.Record())
	}
	if engine.Removed || engine.Resources[engine.CreateID] != o || slices.Contains(engine.Events, "SIGTERM") || slices.Contains(engine.Events, "SIGKILL") {
		t.Fatal("failed exit publication removed or signalled the exited worker", engine.Events)
	}
	// Once storage recovers, fresh owned reconciliation can record the exit
	// and finish cleanup without signalling an already exited primary.
	registry.BeforeSave, registry.SaveFailure = nil, ""
	if failure = stopSession(ctx, s, c, "SIGTERM", nil, true); failure != "" || s.Record().State != "stopped" || s.Record().WorkloadExit == nil || *s.Record().WorkloadExit != 17 || len(engine.Resources) != 0 {
		t.Fatal("retry lost the observed exit or failed owned cleanup", failure, s.Record())
	}
}

func TestForegroundTerminationReobservesExitAfterLostGraceWait(t *testing.T) {
	s, c, _, engine := runningRecoveryFixture(t)
	waits := 0
	engine.After = func(event string) {
		if event == "wait" {
			waits++
			engine.WaitFailure = port.ExecutionFailed
			if waits == 2 {
				engine.WaitFailure = ""
			}
		}
	}
	ctx := context.WithValue(t.Context(), runGraceKey{}, time.Now().Add(10*time.Second))
	failure := terminateSession(ctx, s, c, "SIGTERM")
	if failure != "" || s.Record().WorkloadExit == nil || *s.Record().WorkloadExit != 17 || waits != 2 {
		t.Fatal("fresh exit observation was lost after the grace wait failed", failure, s.Record(), waits)
	}
	if engine.Resources[engine.CreateID].Running || engine.Removed || slices.Contains(engine.Events, "SIGKILL") {
		t.Fatal("exit reobservation escalated or removed the worker", engine.Events)
	}
}

func TestForegroundTerminationCannotEscalateBeyondTeardownDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, c, _, engine := runningRecoveryFixture(t)
		engine.IgnoreSignal, engine.WaitFailure = true, port.ExecutionFailed
		// The wait transport fails while independent inspection still reports
		// a live worker. That error cannot shorten grace or extend teardown.
		engine.AfterWait = func() {
			o := engine.Resources[engine.CreateID]
			o.Status, o.Running, o.PID = "running", true, 123
			engine.Resources[engine.CreateID] = o
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = context.WithValue(ctx, runGraceKey{}, time.Now().Add(10*time.Second))
		start := time.Now()
		failure := terminateSession(ctx, s, c, "SIGTERM")
		if failure != port.Incomplete || time.Since(start) != 5*time.Second || s.Record().WorkloadExit != nil {
			t.Fatal("grace outlived teardown or invented an exit", failure, time.Since(start), s.Record())
		}
		if !engine.Resources[engine.CreateID].Running || engine.Removed || slices.Contains(engine.Events, "SIGKILL") {
			t.Fatal("expired teardown mutated the live worker", engine.Events)
		}
	})
}
