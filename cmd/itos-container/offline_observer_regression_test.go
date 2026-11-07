package main

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

// Force cleanup to arrive before the drain goroutine is scheduled. The fixture
// must establish its drain-before-reopen schedule with real calls, not reorder
// its recorded trace or rely on which goroutine the scheduler happens to run.
func TestOfflineObserverCleanupWaitsForDrain(t *testing.T) {
	for _, failure := range []sp.Outcome{"", sp.ExecutionFailed} {
		t.Run(string(failure), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				o, attachment := offlineObserverAttachment(t, failure)
				registry := offlineRegistry{o: o}
				done := make(chan sp.Outcome, 1)
				go func() {
					_, f := registry.Open(context.Background(), "unused", false)
					done <- f
				}()
				synctest.Wait()
				for _, event := range o.events {
					if event.Operation == "reopen" {
						t.Error("cleanup reopened before the fixture drain completed")
					}
				}
				if f := attachment.Drain(context.Background()); f != failure {
					t.Fatalf("drain failure = %q, want %q", f, failure)
				}
				if f := <-done; f != "slice-13-private-adapter-failure" {
					t.Fatalf("cleanup failure = %q", f)
				}
				last := o.events[len(o.events)-2:]
				if last[0].Operation != "drain" || last[0].Failure != string(failure) || last[1].Operation != "reopen" || last[1].Failure != "slice-13-private-adapter-failure" {
					t.Fatalf("lost actual drain/cleanup observations: %+v", last)
				}
			})
		})
	}
}

func TestOfflineObserverMissingDrainIsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, attachment := offlineObserverAttachment(t, "")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		start := time.Now()
		_, failure := (offlineRegistry{o: o}).Open(ctx, "unused", false)
		if failure != sp.Incomplete || time.Since(start) != time.Second {
			t.Errorf("missing drain returned %q after %s", failure, time.Since(start))
		}
		for _, event := range o.events {
			if event.Operation == "reopen" {
				t.Error("missing drain allowed cleanup to run")
			}
		}
		// Late completion remains a real observation and cannot resume cleanup.
		if f := attachment.Drain(context.Background()); f != "" {
			t.Fatal(f)
		}
		if last := o.events[len(o.events)-1]; last.Operation != "drain" {
			t.Fatalf("late completion = %+v", last)
		}
	})
}

func offlineObserverAttachment(t *testing.T, failure sp.Outcome) (*offlineObserver, sp.Attachment) {
	t.Helper()
	binding, _, engine := fake.Fixture()
	o := &offlineObserver{ops: &sessionContractIO{}, request: sessionContractRequest{CleanupCause: "internal"}}
	c, f := (offlineEngine{RunEngine: &fake.RunEngine{Engine: engine, DrainFailure: failure}, o: o}).OpenRun(context.Background(), binding)
	if f != "" {
		t.Fatal(f)
	}
	t.Cleanup(c.Close)
	a, f := c.Attach(context.Background(), "unused", sp.Streams{})
	if f != "" {
		t.Fatal(f)
	}
	t.Cleanup(a.Close)
	o.exited = true
	return o, a
}
