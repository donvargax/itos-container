package domain_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func runnerFixture() (domain.Runner, approval.Durable, *st.Store, *st.RunEngine) {
	b, store, engine := st.Fixture()
	run := &st.RunEngine{Engine: engine}
	return domain.Runner{Store: store, Engine: run, Entropy: st.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &st.Workspace{}}, approval.Durable{Frozen: b.Frozen}, store, run
}
func TestForegroundLifecycle(t *testing.T) {
	s, d, store, engine := runnerFixture()
	engine.Exit = 7
	argv := []string{"probe", "", "a b", "$HOME", "--"}
	engine.After = func(event string) {
		if event != "start" {
			return
		}
		ref := port.Reference(d.Frozen.Project, strings.Repeat("1", 64))
		r, f := store.Codec.Decode(store.Records[ref])
		if f != "" || r.Stage != "owned" || r.ContainerID != engine.CreateID {
			t.Fatal("start without durable identity")
		}
	}
	result := s.Run(context.Background(), d, argv, port.Streams{})
	if result.Outcome != "" || result.Cleanup != port.Complete || !result.Exited || result.Exit != 7 || result.ID != engine.CreateID {
		t.Fatal(result)
	}
	if !slices.Equal(argv, engine.Argv) {
		t.Fatal("argv changed")
	}
	want := []string{"open", "create", "verify", "attach", "verify", "start", "wait", "drain", "open", "remove", "detach"}
	if !slices.Equal(engine.Events, want) {
		t.Fatalf("events: %v", engine.Events)
	}
}

func TestForegroundRefusalsNeverStart(t *testing.T) {
	for _, fault := range []string{"entropy", "intent", "workspace", "engine", "ownership", "boundary", "inspection", "attach", "changed workspace", "foreign identity", "unavailable inspect", "invalid create id"} {
		t.Run(fault, func(t *testing.T) {
			s, d, store, e := runnerFixture()
			switch fault {
			case "entropy":
				s.Entropy = st.Entropy{Failure: port.IdentityFailed}
			case "intent":
				store.Fault = "intent file fsync"
			case "workspace":
				s.Workspace = &st.Workspace{Failure: port.Refused}
			case "engine":
				e.Unavailable = true
			case "ownership":
				store.Fault = "owned-record file fsync"
			case "boundary":
				e.Boundary = port.IsolationUnverified
			case "inspection":
				e.Boundary = port.IsolationInspection
			case "attach":
				e.AttachFailure = port.ExecutionFailed
			case "changed workspace":
				e.After = func(event string) {
					if event == "attach" {
						s.Workspace.(*st.Workspace).Changed = true
					}
				}
			case "foreign identity":
				e.Inspection = "different full container ID"
			case "unavailable inspect":
				e.Inspection = "unavailable"
			case "invalid create id":
				e.CreateID = "short"
			}
			r := s.Run(context.Background(), d, []string{"probe"}, port.Streams{})
			if r.Outcome == "" || r.Exited || r.ID != "" || slices.Contains(e.Events, "start") {
				t.Fatalf("unsafe handoff: %#v %v", r, e.Events)
			}
			if (fault == "boundary" || fault == "inspection" || fault == "attach" || fault == "changed workspace") && r.Cleanup != port.Complete {
				t.Fatalf("owned unstarted cleanup failed: %#v", r)
			}
		})
	}
}

func TestForegroundRetainsExitAndCleanupSeparately(t *testing.T) {
	for _, exit := range []int{0, 1, 2, 3, 7, 70, 75} {
		s, d, _, e := runnerFixture()
		e.Exit = exit
		e.Following = "still present"
		r := s.Run(context.Background(), d, []string{"probe"}, port.Streams{})
		if !r.Exited || r.Exit != exit || r.Cleanup == port.Complete || r.Reference == "" {
			t.Fatal(r)
		}
	}
}

func TestForegroundUnknownExitIsNotSuccess(t *testing.T) {
	for _, phase := range []string{"start", "wait", "drain"} {
		s, d, _, e := runnerFixture()
		switch phase {
		case "start":
			e.StartFailure = port.ExecutionFailed
		case "wait":
			e.WaitFailure = port.ExecutionFailed
		case "drain":
			e.DrainFailure = port.ExecutionFailed
		}
		r := s.Run(context.Background(), d, []string{"probe"}, port.Streams{})
		if r.Outcome != port.ExecutionFailed || r.Exited != (phase == "drain") || r.Cleanup != port.Complete {
			t.Fatal(phase, r)
		}
	}
}

func TestRunCleanupRetainsNameOnlyNotFoundUntilDelayedCreateAppears(t *testing.T) {
	s, d, store, e := runnerFixture()
	e.Fault, e.Following = "delayed create", "explicit not-found"
	r := s.Run(context.Background(), d, []string{"probe"}, port.Streams{})
	if r.Outcome != port.Uncertain || r.Cleanup != port.Uncertain || r.Exited {
		t.Fatal("name-only absence discarded uncertain create", r)
	}
	record, failure := store.Codec.Decode(store.Records[r.Reference])
	if failure != "" || record.Stage != "intent" || record.ContainerID != "" || slices.Contains(e.Events, "remove") {
		t.Fatal("uncertain intent was changed", record, e.Events)
	}
	e.Fault, e.Following = "", ""
	if recovered := s.Cleanup(context.Background(), r.Reference); recovered.Outcome != port.Complete {
		t.Fatal("delayed worker could not be recovered", recovered)
	}
	creates := 0
	for _, event := range e.Events {
		if event == "create" {
			creates++
		}
	}
	if creates != 1 || slices.Contains(e.Events, "start") {
		t.Fatal("recovery launched or duplicated the worker", e.Events)
	}
}
