package domain

import (
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func freshEgressRunner() (Runner, ap.Durable, *f.Registry, *f.RunEngine, *f.RefusalLog) {
	b, store, engine := f.Fixture()
	b = f.EgressBinding(b)
	store.Codec.(*f.Codec).Value = b
	registry := &f.Registry{Store: store, Project: b.Frozen.Project, Admissions: &f.Admissions{}}
	run := &f.RunEngine{Engine: engine, Exit: 17, ProxyOutput: f.ProxyStdout, ProxyErrorOutput: f.ProxyStderr}
	log := &f.RefusalLog{}
	runner := Runner{Store: store, Registry: registry, Engine: run, Workspace: f.MountWorkspace(b), Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Refusals: log}
	return runner, ap.Durable{Frozen: b.Frozen}, registry, run, log
}
func TestEgressForegroundPreservesExitAndReports(t *testing.T) {
	for _, fault := range []string{"clean", "gap", "copy failure", "proxy exited", "watch unavailable"} {
		t.Run(fault, func(t *testing.T) {
			runner, d, registry, engine, log := freshEgressRunner()
			want := port.Outcome("")
			switch fault {
			case "gap":
				log.Report = port.RefusalReport{Outcome: port.RefusalLogGap, Gaps: []port.RefusalGap{{First: 2, Last: 2}}}
				engine.ProxyOutput = strings.Replace(f.ProxyStdout, `"seq":2`, `"seq":3`, 1)
			case "copy failure":
				log.Fault = "commit"
			case "proxy exited":
				done := make(chan struct{})
				close(done)
				engine.ProxyExit = done
				engine.WaitUntilCancel = true
				want = port.ProxyExited
			case "watch unavailable":
				engine.AfterStart = func() { engine.OpenFailure = port.EngineLost }
				want = port.EngineLost
			}
			result := runner.Run(t.Context(), d, []string{"probe"}, port.Streams{})
			if fault == "watch unavailable" {
				engine.OpenFailure = ""
				recovered := (Recovery{Registry: registry, Engine: engine, Refusals: log}).Stop(t.Context(), result.SessionID)
				if len(recovered.Failures) != 0 {
					t.Fatal("watch failure cleanup retry", recovered)
				}
			}
			if result.Outcome != want {
				t.Errorf("run outcome = %#v, want %s", result, want)
			}
			if fault != "watch unavailable" && (!result.Exited || result.Exit != 17) {
				t.Errorf("workload exit lost: %#v", result)
			}
			if fault == "copy failure" {
				if result.Cleanup != port.Incomplete || len(engine.Resources) != 1 || len(engine.Networks) != 2 {
					t.Error("copy failure lost resources", result)
				}
				log.Fault = ""
				retry := (Recovery{Registry: registry, Engine: engine, Refusals: log}).Stop(t.Context(), result.SessionID)
				if len(retry.Failures) != 0 {
					t.Error("copy retry", retry)
				}
			} else if fault != "watch unavailable" && result.Cleanup != port.Complete {
				t.Errorf("cleanup did not complete: %#v", result)
			}
			if !reflect.DeepEqual(result.RefusalLog, log.Report) {
				t.Errorf("log report lost: %#v", result)
			}
			if len(engine.Resources) != 0 || len(engine.Networks) != 0 || len(engine.Anchors) != 0 {
				t.Errorf("resources remain: %v", engine.Events)
			}
			if string(log.Copies[result.SessionID]) != engine.ProxyOutput {
				t.Errorf("copy changed bytes: %q", log.Copies[result.SessionID])
			}
			b := registry.Store.Codec.(*f.Codec).Value
			child := &f.Registry{Store: &f.Store{Root: b.Checkout.Root + "/child"}, Project: b.Frozen.Project, Admissions: registry.Admissions}
			lease, failure := child.Register(t.Context(), d, strings.Repeat("3", 64), strings.Repeat("4", 64))
			if failure != "" {
				t.Errorf("completed cleanup retained admission: %s", failure)
			} else {
				lease.Close()
			}
			if fault == "proxy exited" && !slices.Contains(engine.Events, "SIGTERM") {
				t.Error("proxy exit did not terminate worker", engine.Events)
			}
		})
	}
}
func TestEgressCleanupKeepsCommittedGap(t *testing.T) {
	runner, registry, engine, id, _ := seedEgressSession(t, 9)
	log := runner.Refusals.(*f.RefusalLog)
	log.Report = port.RefusalReport{Outcome: port.RefusalLogGap, Gaps: []port.RefusalGap{{First: 2, Last: 2}}}
	got := runner.Cleanup(t.Context(), registry.Records[id].Intent)
	if got.Outcome != port.Complete || !reflect.DeepEqual(got.RefusalLog, log.Report) || len(engine.Resources) != 0 || len(engine.Networks) != 0 {
		t.Errorf("cleanup lost gap or retained resources: %#v", got)
	}
}
func TestEgressGapAndRemovalFailureRemainSeparate(t *testing.T) {
	runner, registry, engine, id, _ := seedEgressSession(t, 9)
	log := runner.Refusals.(*f.RefusalLog)
	log.Report = port.RefusalReport{Outcome: port.RefusalLogGap, Gaps: []port.RefusalGap{{First: 2, Last: 2}}}
	engine.EgressFault = "egress-network remove"
	result := (Recovery{Registry: registry, Engine: engine, Refusals: log}).Stop(t.Context(), id)
	if len(result.Failures) != 2 || result.Failures[0].Outcome != port.RefusalLogGap || result.Failures[1].Outcome != port.Incomplete || len(result.Entries) != 1 || len(result.Entries[0].Retry) == 0 || len(engine.Resources) != 0 || len(engine.Networks) != 2 {
		t.Errorf("gap hid removal failure: %#v", result)
	}
	engine.EgressFault = ""
	result = (Recovery{Registry: registry, Engine: engine, Refusals: log}).Stop(t.Context(), id)
	if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" || len(log.Copies[id]) == 0 {
		t.Errorf("remaining cleanup lost published log: %#v", result)
	}
}
