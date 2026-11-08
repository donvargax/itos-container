package domain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type detectorWorkspace struct {
	*f.Workspace
	process                  *f.Process
	open                     func()
	stat                     func()
	openFailure, statFailure port.Outcome
	nilProcess               bool
}

func (w *detectorWorkspace) HoldProcess(int) (port.Process, port.Outcome) {
	if w.open != nil {
		w.open()
	}
	if w.nilProcess {
		return nil, w.openFailure
	}
	return w.process, w.openFailure
}
func (w *detectorWorkspace) Mounted(pid int) (port.MountSource, port.Outcome) {
	if w.stat != nil {
		w.stat()
	}
	if w.statFailure != "" {
		return port.MountSource{}, w.statFailure
	}
	return w.Workspace.Mounted(pid)
}

type noProcessWorkspace struct{ port.AnchorWorkspace }

func TestAnchorDetectorProcessRaces(t *testing.T) {
	for _, name := range []string{"healthy", "exited", "never started", "paused", "missing state", "invalid pid", "no process port", "open failed", "nil process", "exit during open", "pid changed", "fresh inspect unavailable", "fresh inspect foreign", "process exited before stat", "same pid reused", "process error", "stat failed while running", "root changed", "root unavailable after exit", "root unavailable with process error", "exit during stat", "process error after stat", "wrong device", "wrong inode"} {
		test := func(t *testing.T) {
			runner, frozen, registry, engine, id := registeredLifecycleFixture()
			w := &detectorWorkspace{Workspace: runner.Workspace.(*f.Workspace), process: &f.Process{}}
			expectedMount := w.Mounts[123]
			runner.Workspace = w
			started, inspections := false, 0
			change := func(fn func(*port.Observation)) {
				for id, o := range engine.Resources {
					fn(&o)
					engine.Resources[id] = o
				}
			}
			exit := func() {
				w.process.Ended = true
				change(func(o *port.Observation) { o.Running, o.Status, o.PID = false, "exited", 0 })
			}
			want := port.DetectorRefused
			switch name {
			case "root changed":
				want = port.DetectorRootChanged
			case "healthy":
				want = port.DetectorVerified
			case "exited", "exit during open", "process exited before stat", "exit during stat", "root unavailable after exit":
				want = port.DetectorExited
			}
			engine.AfterStart = func() {
				started = true
				switch name {
				case "exited":
					exit()
				case "never started":
					change(func(o *port.Observation) { o.Running, o.Started, o.Status = false, false, "exited" })
				case "paused":
					change(func(o *port.Observation) { o.Running, o.Status = false, "paused" })
				case "missing state":
					change(func(o *port.Observation) { o.StatePresent = false })
				case "invalid pid":
					change(func(o *port.Observation) { o.PID = 0 })
				}
			}
			engine.BeforeInspect = func() {
				if !started {
					return
				}
				inspections++
				engine.Inspection = ""
				if inspections == 2 {
					switch name {
					case "fresh inspect unavailable":
						engine.Inspection = "unavailable"
					case "fresh inspect foreign":
						engine.Inspection = "different full container ID"
					}
				}
			}
			engine.AfterInspect = func() {
				if inspections == 1 && name == "missing state" {
					change(func(o *port.Observation) { o.StatePresent = true })
				}
				if inspections == 1 && (name == "never started" || name == "paused") {
					change(func(o *port.Observation) { o.Running, o.Started, o.Status = true, true, "running" })
				}
				if inspections == 2 && name == "process exited before stat" {
					exit()
				}
				if inspections == 2 && name == "same pid reused" {
					w.process.Ended = true
					engine.WaitUntilCancel = true
				}
			}
			switch name {
			case "no process port":
				runner.Workspace = noProcessWorkspace{w.Workspace}
			case "open failed":
				w.openFailure, w.nilProcess = port.MountRefused, true
			case "nil process":
				w.nilProcess = true
			case "exit during open":
				w.open, w.openFailure, w.nilProcess = exit, port.MountRefused, true
			case "pid changed":
				w.open = func() { change(func(o *port.Observation) { o.PID++ }) }
			case "process error":
				w.process.Failure = port.MountRefused
			case "root changed":
				w.statFailure = port.MountRootUnavailable
			case "root unavailable after exit":
				w.statFailure, w.stat = port.MountRootUnavailable, exit
			case "root unavailable with process error":
				w.statFailure = port.MountRootUnavailable
				w.stat = func() { w.process.Failure = port.MountRefused }
			case "stat failed while running":
				w.statFailure = port.MountRefused
			case "exit during stat":
				w.stat = exit
			case "process error after stat":
				w.stat = func() { w.process.Failure = port.MountRefused }
			case "wrong device":
				w.Mounts[123] = port.MountSource{Device: "different", Inode: w.Mounts[123].Inode}
			case "wrong inode":
				w.Mounts[123] = port.MountSource{Device: w.Mounts[123].Device, Inode: "different"}
			}
			result := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{})
			r := registry.Records[id]
			if r.MountDetector != want || result.Cleanup != port.Complete || len(engine.Resources) != 0 || len(engine.Anchors) != 0 {
				t.Fatal(result, r)
			}
			if want == port.DetectorRefused {
				if result.Outcome != port.MountRefused {
					t.Fatal(result)
				}
				checks := map[string]string{
					"missing state": "worker-state",
					"never started": "worker-init-pid", "paused": "worker-init-pid", "invalid pid": "worker-init-pid",
					"no process port": "pidfd-open", "open failed": "pidfd-open", "nil process": "pidfd-open",
					"pid changed": "worker-pid-recheck", "fresh inspect unavailable": "worker-inspect", "fresh inspect foreign": "worker-ownership",
					"same pid reused": "pidfd-before-stat", "process error": "pidfd-before-stat", "stat failed while running": "mounted-stat", "root unavailable with process error": "mounted-stat",
					"process error after stat": "pidfd-after-stat", "wrong device": "mounted-identity", "wrong inode": "mounted-identity",
				}
				if result.MountDiagnostic.Check != checks[name] || result.MountDiagnostic.ExpectedDevice != expectedMount.Device || result.MountDiagnostic.ExpectedInode != expectedMount.Inode {
					t.Fatal("lost refusal reason or expected identity", name, result.MountDiagnostic)
				}
			} else if result.Outcome != "" || !result.Exited || result.Exit != 17 || r.WorkloadExit == nil || *r.WorkloadExit != 17 {
				t.Fatal(result, r)
			} else if result.MountDiagnostic != (port.MountDiagnostic{}) {
				t.Fatal("successful or exited worker retained a refusal diagnostic", result)
			}
		}
		t.Run(name, func(t *testing.T) { synctest.Test(t, test) })
	}
}

func TestFiniteProcessReleasesLateAcquisition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		p := &f.Process{}
		w := &detectorWorkspace{process: p, open: func() { <-release }}
		gate := make(chan struct{}, 1)
		gate <- struct{}{}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		finite := finiteWorkspace{Workspace: w, ctx: ctx, gate: gate}
		if process, failure := finite.HoldProcess(123); process != nil || failure != port.MountRefused {
			t.Fatal(process, failure)
		}
		close(release)
		synctest.Wait()
		if !p.Closed {
			t.Fatal("late pidfd leaked")
		}
	})
}

func TestFiniteProcessKeepsHandleUntilActiveObservationReturns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		process := &f.Process{}
		gate := make(chan struct{}, 1) // An in-flight stat owns the workspace gate.
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		p := finiteProcess{Process: process, ctx: ctx, gate: gate}
		if exited, failure := p.Exited(); exited || failure != port.MountRefused {
			t.Fatal("timed-out process check supplied authority", exited, failure)
		}
		p.Close()
		synctest.Wait()
		if process.Closed {
			t.Fatal("closed process reference during an active observation")
		}
		gate <- struct{}{}
		synctest.Wait()
		if !process.Closed {
			t.Fatal("process reference leaked after observation returned")
		}
	})
}
