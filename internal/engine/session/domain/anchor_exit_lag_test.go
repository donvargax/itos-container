package domain

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestAnchorDetectorDelayedExitReconciliation(t *testing.T) {
	for _, name := range []string{"absent pid", "held process exited", "exit during stat", "wait timeout", "wait failure", "still running after wait", "changed PID before wait", "foreign worker after wait", "inspection unavailable after wait", "exited inspection without started proof", "unregistered run", "absent pid changed", "absent pid inspection unavailable", "ended reference observation failed", "wait ignores cancellation"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runner, frozen, registry, engine, id := registeredLifecycleFixture()
				w := &detectorWorkspace{Workspace: runner.Workspace.(*f.Workspace), process: &f.Process{Ended: true}}
				runner.Workspace = w
				change := func(fn func(*port.Observation)) {
					for id, o := range engine.Resources {
						fn(&o)
						engine.Resources[id] = o
					}
				}
				started, inspections, waits := false, 0, 0
				engine.AfterStart = func() { started = true }
				engine.BeforeInspect = func() {
					if started {
						inspections++
					}
					if (inspections == 3 && name == "changed PID before wait") || (inspections == 2 && name == "absent pid changed") {
						change(func(o *port.Observation) { o.PID++ })
					}
					if inspections == 2 && name == "absent pid inspection unavailable" {
						engine.Inspection = "unavailable"
					}
				}
				engine.After = func(event string) {
					if event == "wait" {
						waits++
						if waits > 1 {
							engine.WaitFailure = port.ExecutionFailed
						}
					}
				}
				engine.AfterWait = func() {
					switch name {
					case "wait ignores cancellation":
						time.Sleep(11 * time.Second)
					case "still running after wait":
						change(func(o *port.Observation) { o.Running, o.Status, o.PID = true, "running", 123 })
					case "foreign worker after wait":
						engine.Inspection = "different full container ID"
					case "inspection unavailable after wait":
						engine.Inspection = "unavailable"
					case "exited inspection without started proof":
						change(func(o *port.Observation) { o.Started = false })
					}
				}
				want := port.DetectorRefused
				switch name {
				case "absent pid":
					w.nilProcess, w.openFailure = true, port.ProcessAbsent
					want = port.DetectorExited
				case "held process exited":
					want = port.DetectorExited
				case "exit during stat":
					w.process.Ended = false
					w.stat = func() { w.process.Ended = true }
					want = port.DetectorExited
				case "unregistered run":
					runner.Registry = nil
					want = port.DetectorExited
				case "wait timeout":
					engine.WaitUntilCancel = true
				case "absent pid changed", "absent pid inspection unavailable":
					w.nilProcess, w.openFailure = true, port.ProcessAbsent
				case "ended reference observation failed":
					w.process.Failure = port.MountRefused
				case "wait failure":
					engine.WaitFailure = port.ExecutionFailed
				}
				start := time.Now()
				result := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{})
				if want == port.DetectorExited {
					if result.Outcome != "" || !result.Exited || result.Exit != 17 || result.Cleanup != port.Complete || waits != 1 || slices.Contains(engine.Events, "SIGKILL") || slices.Contains(engine.Events, "SIGTERM") {
						t.Fatal("confirmed exit was lost or reacquired", result, engine.Events)
					}
					if runner.Registry != nil && (registry.Records[id].WorkloadExit == nil || *registry.Records[id].WorkloadExit != 17) {
						t.Fatal("known exit was not published", registry.Records[id])
					}
				} else if result.Outcome != port.MountRefused || result.Exited {
					t.Fatal("unconfirmed exit supplied success", result)
				}
				if runner.Registry != nil && registry.Records[id].MountDetector != want {
					t.Fatal("wrong detector observation", registry.Records[id])
				}
				if (name == "wait timeout" || name == "wait ignores cancellation") && time.Since(start) != 10*time.Second {
					t.Fatal("exit reconciliation exceeded its single call budget", time.Since(start))
				}
				if (name == "changed PID before wait" || name == "absent pid changed" || name == "absent pid inspection unavailable" || name == "ended reference observation failed") && waits != 0 {
					t.Fatal("PID mismatch reached exit wait")
				}
				if !w.nilProcess && !w.process.Closed {
					t.Fatal("process reference leaked")
				}
				if name == "wait ignores cancellation" {
					// Let the abandoned adapter call return after measuring the
					// controller's bound, then verify its late reply stayed unused.
					time.Sleep(time.Second)
					synctest.Wait()
					if registry.Records[id].WorkloadExit != nil {
						t.Fatal("late wait reply supplied a workload result")
					}
				}
			})
		})
	}
}
