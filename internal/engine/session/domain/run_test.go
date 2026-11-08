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
	return domain.Runner{Store: store, Engine: run, Entropy: st.Entropy{Value: strings.Repeat("1", 64)}, Workspace: st.MountWorkspace(b)}, approval.Durable{Frozen: b.Frozen}, store, run
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
	want := []string{"open", "anchor allocate", "anchor inspect", "anchor inspect", "create", "verify", "attach", "verify", "anchor inspect", "start", "wait", "drain", "open", "remove", "anchor inspect", "anchor inspect", "anchor remove", "anchor inspect", "detach"}
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

func TestRunRevalidatesBeforeCreateAndStart(t *testing.T) {
	for _, barrier := range []string{"open", "attach"} {
		for _, fault := range []string{"record", "workspace", "daemon", "boundary"} {
			if barrier == "open" && fault == "boundary" {
				continue // Boundary verification begins after creation.
			}
			t.Run(barrier+"/"+fault, func(t *testing.T) {
				s, d, store, e := runnerFixture()
				want := port.Refused
				e.After = func(event string) {
					if event != barrier {
						return
					}
					switch fault {
					case "record":
						store.UnsafeRecord = true
						want = port.Unsafe
					case "workspace":
						s.Workspace.(*st.Workspace).Changed = true
					case "daemon":
						e.Replaced = true
					case "boundary":
						e.Boundary = port.IsolationUnverified
						want = port.IsolationUnverified
					}
				}
				r := s.Run(context.Background(), d, []string{"probe"}, port.Streams{})
				if r.Outcome != want || r.Exited || r.ID != "" || slices.Contains(e.Events, "start") {
					t.Fatalf("changed authority reached workload: %+v, %v", r, e.Events)
				}
				attached := barrier == "attach"
				if slices.Contains(e.Events, "create") != attached || slices.Contains(e.Events, "detach") != attached {
					t.Fatalf("wrong refusal boundary or attachment leak: %v", e.Events)
				}
				if store.Locked || !s.Workspace.(*st.Workspace).Closed {
					t.Fatal("launch retained intent or workspace handles")
				}
			})
		}
	}
}

func TestRunCleanupRequiresCurrentAuthorityAndObservedAbsence(t *testing.T) {
	for _, tc := range []struct {
		fault  string
		want   port.Outcome
		remove bool
	}{
		{"healthy", port.Complete, true},
		{"intent only", port.Complete, true},
		{"already absent", port.Complete, false},
		{"record after open", port.Unsafe, false},
		{"daemon after open", port.Refused, false},
		{"foreign name-only worker", port.Refused, false},
		{"socket during inspect", port.Refused, false},
		{"inspection unavailable", port.Incomplete, false},
		{"missing state", port.Incomplete, false},
		{"foreign ID", port.Refused, false},
		{"completed record with live worker", port.Refused, false},
		{"ownership write", port.OwnershipWrite, false},
		{"record after sync", port.Unsafe, false},
		{"daemon after sync", port.Refused, false},
		{"socket after remove", port.Refused, true},
		{"absence unavailable", port.Incomplete, true},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			s, d, store, e := runnerFixture()
			ctx := context.Background()
			intent, failure := store.New(ctx, d, strings.Repeat("1", 64))
			if failure != "" {
				t.Fatal(failure)
			}
			record := intent.Record()
			e.Resources = map[string]port.Observation{e.CreateID: {
				ID: e.CreateID, Name: record.Name, ImageID: record.ImageID,
				Token: record.NameToken(), Project: record.Project, Fingerprint: record.Fingerprint,
				StatePresent: true, Status: "exited",
			}}
			if tc.fault != "intent only" && tc.fault != "foreign name-only worker" {
				record.Stage, record.ContainerID = "owned", e.CreateID
				if tc.fault == "completed record with live worker" {
					record.Stage = "absent"
				}
				if failure = intent.Save(record); failure != "" {
					t.Fatal(failure)
				}
			}
			intent.Close()
			switch tc.fault {
			case "already absent":
				delete(e.Resources, e.CreateID)
			case "record after open":
				e.AfterOpen = func() { store.UnsafeRecord = true }
			case "daemon after open":
				e.AfterOpen = func() { e.Replaced = true }
			case "foreign name-only worker":
				e.Inspection = "different name"
			case "socket during inspect":
				e.BeforeInspect = func() { e.Replaced = true }
			case "inspection unavailable":
				e.Inspection = "unavailable"
			case "missing state":
				e.Inspection = "missing State"
			case "foreign ID":
				e.Inspection = "different full container ID"
			case "ownership write":
				store.Fault = "owned-record file fsync"
			case "record after sync":
				store.AfterSave = func() { store.UnsafeRecord = true }
			case "daemon after sync":
				store.AfterSave = func() { e.Replaced = true }
			case "socket after remove":
				e.BeforeInspect = func() {
					if e.Removed {
						e.Replaced = true
					}
				}
			case "absence unavailable":
				e.Following = "unavailable"
			}
			ref := port.Reference(record.Project, record.NameToken())
			got := s.Cleanup(ctx, ref)
			if got.Outcome != tc.want || got.Reference != ref || slices.Contains(e.Events, "remove") != tc.remove {
				t.Fatalf("cleanup = %+v, events %v; want %s, remove %v", got, e.Events, tc.want, tc.remove)
			}
			saved, failure := store.Codec.Decode(store.Records[ref])
			if failure != "" || store.Locked {
				t.Fatal("cleanup lost the recovery record or retained its lock", failure)
			}
			if tc.want == port.Complete {
				if saved.Stage != "absent" || saved.ContainerID != e.CreateID || len(e.Resources) != 0 {
					t.Fatal("cleanup completed without recorded, observed absence", saved)
				}
			} else if saved != record {
				t.Fatal("failed cleanup changed recovery authority", saved, record)
			}
			if slices.Contains(e.Events, "start") || slices.Contains(e.Events, "create") {
				t.Fatal("cleanup launched a worker", e.Events)
			}
		})
	}
}
