package domain

import (
	"context"
	"slices"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestStopRetainsUncertaintyWhenAuthorityOrDurabilityFails(t *testing.T) {
	for _, tc := range []struct {
		name string
		want port.Outcome
	}{
		{"session changed", port.Refused},
		{"daemon changed before inspect", port.Refused},
		{"daemon changed during inspect", port.Refused},
		{"exit write failed", port.IntentWrite},
		{"stopping write failed", port.IntentWrite},
		{"daemon changed before remove", port.Refused},
		{"absence write failed", port.CompletionWrite},
		{"budget expired after open", port.Incomplete},
		{"removal inspection unavailable", port.Incomplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, registry, engine, _, id := registeredSessionFixture(t)
			c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			defer a.Close()
			engine.Events = nil
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			original := registry.Records[id]
			var exit *int
			switch tc.name {
			case "session changed":
				registry.AfterOpen = func(string) {
					r := registry.Records[id]
					r.State = "failed"
					registry.Records[id] = r
				}
			case "daemon changed before inspect":
				engine.AfterOpen = func() { engine.Replaced = true }
			case "daemon changed during inspect":
				engine.BeforeInspect = func() { engine.Replaced = true }
			case "exit write failed":
				n := 7
				exit = &n
				registry.SaveFailure = port.IntentWrite
			case "stopping write failed":
				registry.SaveFailure = port.IntentWrite
			case "daemon changed before remove", "removal inspection unavailable":
				o := engine.Resources[worker]
				o.Status, o.Running, o.Started, o.PID = "created", false, false, 0
				engine.Resources[worker] = o
				if tc.name == "daemon changed before remove" {
					registry.Store.AfterSave = func() {
						engine.AfterInspect = func() { engine.Replaced = true }
					}
				} else {
					engine.Following = "unavailable"
				}
			case "absence write failed":
				delete(engine.Resources, worker)
				registry.Store.Fault = "absent-record file fsync"
			case "budget expired after open":
				engine.AfterOpen = cancel
			}

			result := (Recovery{Registry: registry, Engine: engine}).stop(ctx, id, "SIGTERM", exit, true)
			if len(result.Failures) != 1 || result.Failures[0].Session != id || result.Failures[0].Outcome != tc.want || len(result.Entries) != 1 {
				t.Fatal("stop lost the boundary failure", result)
			}
			e := result.Entries[0]
			wantResource := "unknown"
			switch tc.name {
			case "exit write failed", "stopping write failed":
				wantResource = "running"
			case "daemon changed before remove":
				wantResource = "created"
			case "absence write failed":
				wantResource = "absent"
			}
			wantEvents := []string{"open"}
			if tc.name != "session changed" && tc.name != "daemon changed before inspect" && tc.name != "budget expired after open" {
				wantEvents = append(wantEvents, "anchor inspect")
			}

			if e.Cleanup != "unknown" || e.Resource != wantResource || e.State != "failed" || e.WorkloadExit != nil || !slices.Equal(e.Retry, []string{"itos", "container", "stop", id}) {
				t.Fatal("refused stop claimed completion or lost its exact retry", e)
			}
			r := registry.Records[id]
			if r.State == "stopped" || r.WorkloadExit != nil || r.Intent != original.Intent || registry.Locks[id] || registry.Store.Locked || engine.Removed != (tc.name == "removal inspection unavailable") || !slices.Equal(engine.Events, wantEvents) {
				t.Fatal("refused stop changed authority, mutated a worker or leaked a lock", r, engine.Events)
			}
			if tc.name == "budget expired after open" && (r != original || registry.Writes != 3) {
				t.Fatal("expired total allowed a progress write", r)
			}
		})
	}
}

func TestTerminationRechecksAfterLostKillWait(t *testing.T) {
	for _, name := range []string{"kill refused", "inspection unavailable", "exited without wait result"} {
		t.Run(name, func(t *testing.T) {
			runner, registry, engine, _, id := registeredSessionFixture(t)
			c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			defer a.Close()
			running := engine.Resources[worker]
			engine.Events = nil
			engine.IgnoreSignal, engine.WaitFailure = true, port.ExecutionFailed
			waits := 0
			engine.After = func(event string) {
				if event != "wait" {
					return
				}
				waits++
				if waits == 1 {
					engine.BeforeInspect = func() { engine.Resources[worker] = running }
					if name == "kill refused" {
						engine.SignalFailure = port.Refused
					}
				} else {
					engine.BeforeInspect = nil
					if name == "inspection unavailable" {
						engine.Inspection = "unavailable"
					}
				}
			}

			result := (Recovery{Registry: registry, Engine: engine}).Stop(t.Context(), id)
			wantEvents := []string{"open", "anchor inspect", "SIGTERM", "wait", "SIGKILL"}
			if name != "kill refused" {
				wantEvents = append(wantEvents, "wait")
			}
			if name == "exited without wait result" {
				wantEvents = append(wantEvents, "anchor inspect", "anchor inspect", "anchor remove", "anchor inspect")
			}
			if !slices.Equal(engine.Events, wantEvents) || registry.Locks[id] || len(result.Entries) != 1 || result.Entries[0].WorkloadExit != nil || registry.Records[id].WorkloadExit != nil {
				t.Fatal("termination repeated signals, leaked locks or invented an exit", result, engine.Events)
			}
			if name == "exited without wait result" {
				if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" || len(engine.Resources) != 0 || registry.Records[id].State != "stopped" {
					t.Fatal("verified nonrunning worker could not complete cleanup", result)
				}
			} else {
				want := port.Incomplete
				if name == "kill refused" {
					want = port.Refused
				}
				if len(result.Failures) != 1 || result.Failures[0].Outcome != want || result.Entries[0].Cleanup != "unknown" || engine.Removed || len(engine.Resources) != 1 {
					t.Fatal("failed kill or observation authorized removal", result)
				}
			}
		})
	}
}
