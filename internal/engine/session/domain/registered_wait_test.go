package domain

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestRegisteredStdinCloseRevalidatesPendingSession(t *testing.T) {
	for _, name := range []string{"verified exit", "substituted record", "session unavailable"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runner, registry, engine, _, id := registeredSessionFixture(t)
				engine.StdinClosed, engine.WaitUntilCancel, engine.Exit = true, true, 7
				c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
				if failure != "" {
					t.Fatal(failure)
				}
				defer c.Close()
				defer a.Close()
				engine.Events = nil
				engine.After = func(event string) {
					if event == "wait" {
						engine.After = nil
						synctest.Wait() // The closed stdin cancels the original wait.
						o := engine.Resources[worker]
						o.Status, o.Running, o.PID = "exited", false, 0
						engine.Resources[worker] = o
					}
				}
				switch name {
				case "substituted record":
					registry.AfterOpen = func(string) {
						r := registry.Records[id]
						r.State = "failed"
						registry.Records[id] = r
					}
				case "session unavailable":
					registry.Failure = port.Incomplete
				}

				exit, exited, failure := runner.waitRun(t.Context(), c, a, id, worker)
				wantEvents := []string{"wait", "open", "wait"}
				if name != "verified exit" {
					wantEvents = []string{"wait"}
					if exit != 0 || exited || failure != port.ExecutionFailed {
						t.Fatal("substituted session supplied exit knowledge", exit, exited, failure)
					}
				} else if exit != 7 || !exited || failure != "" {
					t.Fatal("fresh original worker exit was not reconciled", exit, exited, failure)
				}
				if !slices.Equal(engine.Events, wantEvents) || registry.Readers[id] != 0 || registry.Locks[id] || registry.Records[id].WorkloadExit != nil || engine.Removed {
					t.Fatal("stdin probe mutated recovery or retained authority", engine.Events)
				}
			})
		})
	}
}

func TestStoppedRunRefusesRecordChangedAfterOpen(t *testing.T) {
	_, registry, _, _, id := registeredSessionFixture(t)
	entry, failure := registry.Open(t.Context(), id, true)
	if failure != "" {
		t.Fatal(failure)
	}
	defer entry.Close()
	r := registry.Records[id]
	r.State = "stopped"
	registry.Records[id] = r
	if exit, exited, failure := stoppedRun(entry, strings.Repeat("c", 64)); exit != 0 || exited || failure != port.ExecutionFailed {
		t.Fatal("changed completion record erased the original wait failure", exit, exited, failure)
	}
}

func TestRegisteredRunWaitsForStopWithinItsExistingTeardownTotal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, store, engine := f.Fixture()
		released := make(chan struct{})
		registry := &f.Registry{Store: store, Project: b.Frozen.Project, WaitForLock: true, LockReleased: released}
		stdin := make(chan port.Outcome, 1)
		e := &f.RunEngine{Engine: engine, StdinFailures: stdin, WaitUntilCancel: true, Exit: 7}
		id := strings.Repeat("1", 64)
		e.After = func(event string) {
			if event != "wait" {
				return
			}
			e.After = nil
			registry.Locks[id] = true
			stdin <- port.StdinClosed
			go func() {
				time.Sleep(11 * time.Second)
				synctest.Wait() // Run is blocked in its teardown lock acquisition.
				registry.Locks[id] = false
				r := (Recovery{Registry: registry, Engine: e}).Stop(context.Background(), id)
				if len(r.Failures) != 0 || len(r.Entries) != 1 || r.Entries[0].Cleanup != "complete" {
					t.Error("competing stop could not finish", r)
				}
				close(released)
			}()
			synctest.Wait()
		}
		runner := Runner{Store: store, Registry: registry, Engine: e, Entropy: f.Entropy{Value: id}, Workspace: &f.Workspace{}}
		start := time.Now()
		result := runner.Run(t.Context(), approval.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
		if time.Since(start) != 11*time.Second || result.Outcome != "" || !result.Exited || result.Exit != 7 || result.Cleanup != port.Complete {
			t.Fatal("late verified completion did not settle the provisional stdin failure", result, time.Since(start))
		}
		if len(engine.Resources) != 0 || registry.Locks[id] || registry.Readers[id] != 0 || registry.Leases[id] || registry.Records[id].State != "stopped" || !slices.Contains(e.Events, "drain") {
			t.Fatal("adopted completion lost drain, cleanup or released authority", e.Events)
		}
	})
}
