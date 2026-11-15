package domain

import (
	"context"
	"reflect"
	"strings"
	"testing"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func serviceFixture() (Service, approval.Durable, *porttest.Store, *porttest.Engine) {
	b, store, engine := porttest.Fixture()
	return Service{Store: store, Engine: engine, Entropy: porttest.Entropy{Value: strings.Repeat("1", 64)}}, approval.Durable{Frozen: b.Frozen}, store, engine
}
func TestOwnedLifecycleAndIndependentRecovery(t *testing.T) {
	s, d, store, engine := serviceFixture()
	ctx := context.Background()
	created := s.Create(ctx, d)
	if created.Outcome != port.Owned || created.ID != engine.CreateID {
		t.Fatal(created)
	}
	if len(store.Records) == 0 {
		t.Fatal("no durable ownership")
	}
	// Reconstruct a controller with no creation reply and no entropy capability.
	cleanup := (Service{Store: store, Engine: engine}).Cleanup(ctx, created.Reference)
	if cleanup.Outcome != port.Complete || cleanup.ID != "" || len(engine.Resources) != 0 {
		t.Fatal(cleanup)
	}
	events := len(engine.Events)
	if again := s.Cleanup(ctx, created.Reference); !reflect.DeepEqual(again, cleanup) {
		t.Fatal(again)
	}
	for _, e := range engine.Events[events:] {
		if e.Method == "REMOVE" || e.Method == "CREATE" {
			t.Fatal("repeat mutation")
		}
	}
}
func TestEntropyAndStorageFailuresPrecedeEngine(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Service, *porttest.Store)
		want  port.Outcome
	}{
		{"entropy", func(s *Service, _ *porttest.Store) { s.Entropy = porttest.Entropy{Failure: port.IdentityFailed} }, port.IdentityFailed},
		{"invalid token", func(s *Service, _ *porttest.Store) { s.Entropy = porttest.Entropy{Value: "../bad"} }, port.IdentityFailed},
		{"unsafe", func(_ *Service, s *porttest.Store) { s.UnsafeRecord = true }, port.Unsafe},
		{"write", func(_ *Service, s *porttest.Store) { s.Fault = "intent short write" }, port.IntentWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, d, store, engine := serviceFixture()
			tc.setup(&s, store)
			r := s.Create(context.Background(), d)
			if r.Outcome != tc.want || r.ID != "" || len(engine.Events) != 0 {
				t.Fatal(r, engine.Events)
			}
		})
	}
}

func TestCreateRetainsIntentWhenAuthorityRevalidationFails(t *testing.T) {
	for _, tc := range []struct {
		name       string
		setup      func(*porttest.Store, *porttest.Engine)
		want       port.Outcome
		operations string
		resources  int
	}{
		{"engine unavailable at open", func(_ *porttest.Store, e *porttest.Engine) {
			e.Unavailable = true
		}, port.Incomplete, "INFO", 0},
		{"foreign daemon at open", func(_ *porttest.Store, e *porttest.Engine) {
			e.DaemonID = "foreign"
		}, port.Refused, "INFO", 0},
		{"unsafe snapshot after open", func(s *porttest.Store, e *porttest.Engine) {
			e.AfterOpen = func() { s.UnsafeSnapshot = true }
		}, port.Unsafe, "INFO", 0},
		{"engine unavailable after open", func(_ *porttest.Store, e *porttest.Engine) {
			e.AfterOpen = func() { e.Unavailable = true }
		}, port.Incomplete, "INFO INFO", 0},
		{"socket replaced after open", func(_ *porttest.Store, e *porttest.Engine) {
			e.AfterOpen = func() { e.Replaced = true }
		}, port.Refused, "INFO INFO", 0},
		{"image declares a volume", func(_ *porttest.Store, e *porttest.Engine) {
			e.ImageInspection = "declared volume"
		}, port.Refused, "INFO INFO IMAGE", 0},
		{"unsafe record after inspection", func(s *porttest.Store, e *porttest.Engine) {
			e.AfterInspect = func() { s.UnsafeRecord = true }
		}, port.Unsafe, "INFO INFO IMAGE CREATE INSPECT", 1},
		{"host identity changed after inspection", func(s *porttest.Store, e *porttest.Engine) {
			e.AfterInspect = func() { s.UID = "2000" }
		}, port.Refused, "INFO INFO IMAGE CREATE INSPECT", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, d, store, engine := serviceFixture()
			binding, failure := store.Codec.Frozen(d.Frozen)
			if failure != "" {
				t.Fatal(failure)
			}
			token := strings.Repeat("1", 64)
			ref := port.Reference(d.Frozen.Project, token)
			original := string(store.Codec.Encode(binding.Intent(token)))
			tc.setup(store, engine)
			got := s.Create(context.Background(), d)
			if !reflect.DeepEqual(got, port.Result{Outcome: tc.want, Reference: ref}) {
				t.Fatalf("create = %+v; want %s with the intent reference and no usable ID", got, tc.want)
			}
			if len(store.Records) != 1 || string(store.Records[ref]) != original || store.Locked {
				t.Fatal("failed creation changed the original intent or retained its lock")
			}
			for _, event := range store.Events {
				if event.Stage != "intent" || event.Failed {
					t.Fatal("failed revalidation attempted to advance ownership", event)
				}
			}
			var operations []string
			for _, event := range engine.Events {
				operations = append(operations, event.Method)
			}
			if strings.Join(operations, " ") != tc.operations || len(engine.Resources) != tc.resources {
				t.Fatalf("engine operations = %v, remaining resources = %d", operations, len(engine.Resources))
			}
		})
	}
}

func TestCreateNeedsMatchingCompleteInspection(t *testing.T) {
	for _, condition := range []string{"unavailable", "explicit not-found", "malformed response", "missing State", "different full container ID", "different name", "different image config ID", "different intent label", "different project label", "different fingerprint label", "running state", "exited state after starting", "created state with nonzero PID", "created state with StartedAt", "mounted volume", "configured volume"} {
		t.Run(condition, func(t *testing.T) {
			s, d, _, e := serviceFixture()
			e.Inspection = condition
			r := s.Create(context.Background(), d)
			want := port.Refused
			if condition == "unavailable" || condition == "explicit not-found" || condition == "malformed response" || condition == "missing State" {
				want = port.Uncertain
			}
			if r.Outcome != want || r.ID != "" || len(e.Resources) != 1 {
				t.Fatal(r)
			}
			for _, event := range e.Events {
				if event.Method == "REMOVE" {
					t.Fatal("removed unverified create")
				}
			}
		})
	}
}
func TestLostCreateRemainsRecoverableAndNeverRetries(t *testing.T) {
	s, d, store, e := serviceFixture()
	e.Fault = "delayed create"
	e.Following = "explicit not-found"
	created := s.Create(context.Background(), d)
	if created.Outcome != port.Uncertain {
		t.Fatal(created)
	}
	first := s.Cleanup(context.Background(), created.Reference)
	if first.Outcome != port.Uncertain {
		t.Fatal(first)
	}
	e.Fault, e.Following = "", ""
	if recovered := (Service{Store: store, Engine: e}).Cleanup(context.Background(), created.Reference); recovered.Outcome != port.Complete {
		t.Fatal(recovered)
	}
	creates := 0
	for _, event := range e.Events {
		if event.Method == "CREATE" {
			creates++
		}
	}
	if creates != 1 {
		t.Fatal(creates)
	}
}
func TestWriteErrorDoesNotBecomeAUsableID(t *testing.T) {
	for _, fault := range []string{"short write", "file fsync", "atomic publication", "parent fsync"} {
		t.Run(fault, func(t *testing.T) {
			s, d, store, e := serviceFixture()
			store.Fault = "owned-record " + fault
			ctx := context.Background()
			created := s.Create(ctx, d)
			if created.Outcome != port.OwnershipWrite || created.ID != "" || len(e.Resources) != 1 {
				t.Fatal(created)
			}
			if result := s.Cleanup(ctx, created.Reference); result.Outcome != port.OwnershipWrite || result.ID != "" {
				t.Fatal(result)
			}
			if len(e.Resources) != 1 {
				t.Fatal("failed ownership persistence removed the worker")
			}
			for _, event := range e.Events {
				if event.Method == "REMOVE" {
					t.Fatal("attempted removal without durable ownership")
				}
			}
			store.Fault = ""
			if result := (Service{Store: store, Engine: e}).Cleanup(ctx, created.Reference); result.Outcome != port.Complete {
				t.Fatal(result)
			}
		})
	}
}

func TestReconciledIDSurvivesFailedCompletion(t *testing.T) {
	for _, fault := range []string{"short write", "file fsync", "atomic publication", "parent fsync"} {
		t.Run(fault, func(t *testing.T) {
			s, d, store, e := serviceFixture()
			e.Fault = "lost create response"
			ctx := context.Background()
			created := s.Create(ctx, d)
			if created.Outcome != port.Uncertain {
				t.Fatal(created)
			}
			store.Fault = "absent-record " + fault
			first := (Service{Store: store, Engine: e}).Cleanup(ctx, created.Reference)
			if first.Outcome != port.CompletionWrite || len(e.Resources) != 0 {
				t.Fatal(first)
			}
			record, failure := store.Codec.Decode(store.Records[created.Reference])
			if failure != "" || record.ContainerID != e.CreateID {
				t.Fatal("lost concrete recovery identity", record, failure)
			}
			store.Fault = ""
			before := len(e.Events)
			if next := (Service{Store: store, Engine: e}).Cleanup(ctx, created.Reference); next.Outcome != port.Complete {
				t.Fatal(next)
			}
			for _, event := range e.Events[before:] {
				if event.Method == "REMOVE" || event.Method == "CREATE" || (event.Method == "INSPECT" && event.Target != e.CreateID) {
					t.Fatal("retry must only inspect the persisted full ID", event)
				}
			}
		})
	}
}
func TestCleanupRefusesBadAuthorityAndHonorsDeadline(t *testing.T) {
	s, d, store, e := serviceFixture()
	ctx := context.Background()
	if r := s.Cleanup(ctx, "../escape"); r.Outcome != port.Invalid || r.Reference != "" {
		t.Fatal(r)
	}
	if r := s.Cleanup(ctx, port.Reference(d.Frozen.Project, strings.Repeat("1", 64))); r.Outcome != port.Refused {
		t.Fatal(r)
	}
	created := s.Create(ctx, d)
	e.DaemonID = "foreign"
	if r := s.Cleanup(ctx, created.Reference); r.Outcome != port.Refused {
		t.Fatal(r)
	}
	e.DaemonID = "engine-A"
	store.UnsafeSnapshot = true
	if r := s.Cleanup(ctx, created.Reference); r.Outcome != port.Unsafe {
		t.Fatal(r)
	}
	store.UnsafeSnapshot = false
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if r := s.Cleanup(canceled, created.Reference); r.Outcome == port.Complete {
		t.Fatal("canceled cleanup completed")
	}
}
func TestRemovalAcknowledgementIsNotAbsence(t *testing.T) {
	for _, following := range []string{"still present", "unavailable", "malformed response"} {
		t.Run(following, func(t *testing.T) {
			s, d, _, e := serviceFixture()
			r := s.Create(context.Background(), d)
			e.Following = following
			result := s.Cleanup(context.Background(), r.Reference)
			if result.Outcome != port.Incomplete {
				t.Fatal(result)
			}
		})
	}
}

func TestCleanupRetainsEvidenceWhenRevalidationFails(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stage   string
		setup   func(*porttest.Store, *porttest.Engine)
		want    port.Outcome
		removes int
	}{
		{"unsafe evidence after open", "owned", func(s *porttest.Store, e *porttest.Engine) {
			e.AfterOpen = func() { s.UnsafeSnapshot = true }
		}, port.Unsafe, 0},
		{"socket replaced after open", "owned", func(_ *porttest.Store, e *porttest.Engine) {
			e.AfterOpen = func() { e.Replaced = true }
		}, port.Refused, 0},
		{"socket replaced before name inspection", "intent", func(_ *porttest.Store, e *porttest.Engine) {
			e.BeforeInspect = func() { e.Replaced = true }
		}, port.Refused, 0},
		{"foreign resource at name", "intent", func(_ *porttest.Store, e *porttest.Engine) {
			e.Inspection = "different name"
		}, port.Refused, 0},
		{"socket replaced before ID inspection", "owned", func(_ *porttest.Store, e *porttest.Engine) {
			e.BeforeInspect = func() { e.Replaced = true }
		}, port.Refused, 0},
		{"ID inspection unavailable", "owned", func(_ *porttest.Store, e *porttest.Engine) {
			e.Inspection = "unavailable"
		}, port.Incomplete, 0},
		{"ID inspection lacks state", "owned", func(_ *porttest.Store, e *porttest.Engine) {
			e.Inspection = "missing State"
		}, port.Incomplete, 0},
		{"name disappears before ID inspection", "intent", func(_ *porttest.Store, e *porttest.Engine) {
			e.AfterInspect = func() { e.Inspection = "explicit not-found" }
		}, port.Uncertain, 0},
		{"foreign resource at ID", "owned", func(_ *porttest.Store, e *porttest.Engine) {
			e.Inspection = "different intent label"
		}, port.Refused, 0},
		{"record unsafe after durable ownership", "owned", func(s *porttest.Store, _ *porttest.Engine) {
			s.AfterSave = func() { s.UnsafeRecord = true }
		}, port.Unsafe, 0},
		{"snapshot unsafe after reconciled ownership", "intent", func(s *porttest.Store, _ *porttest.Engine) {
			s.AfterSave = func() { s.UnsafeSnapshot = true }
		}, port.Unsafe, 0},
		{"socket replaced after inspection", "owned", func(_ *porttest.Store, e *porttest.Engine) {
			e.Inspection = "replace socket"
		}, port.Refused, 0},
		{"socket replaced after removal", "owned", func(_ *porttest.Store, e *porttest.Engine) {
			e.BeforeInspect = func() {
				if e.Removed {
					e.Replaced = true
				}
			}
		}, port.Refused, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, store, engine := porttest.Fixture()
			token, id := strings.Repeat("1", 64), engine.CreateID
			record := b.Intent(token)
			record.Stage = tc.stage
			if tc.stage == "owned" {
				record.ContainerID = id
			}
			ref := port.Reference(b.Frozen.Project, token)
			store.Records[ref] = store.Codec.Encode(record)
			resource := port.Observation{ID: id, Name: record.Name, ImageID: record.ImageID, Token: token,
				Project: record.Project, Fingerprint: record.Fingerprint, StatePresent: true, Status: "created"}
			engine.Resources[id] = resource
			tc.setup(store, engine)
			got := (Service{Store: store, Engine: engine}).Cleanup(context.Background(), ref)
			if !reflect.DeepEqual(got, port.Result{Outcome: tc.want, Reference: ref}) {
				t.Fatalf("cleanup = %+v, want %s with the surviving reference", got, tc.want)
			}
			// Reconciliation may durably advance intent to owned before a later
			// revalidation fails, but no refusal may publish an absent tombstone.
			if tc.name == "snapshot unsafe after reconciled ownership" {
				record.Stage, record.ContainerID = "owned", id
			}
			if len(store.Records) != 1 || string(store.Records[ref]) != string(store.Codec.Encode(record)) || store.Locked {
				t.Fatal("cleanup lost recovery evidence or retained its lock")
			}
			for _, event := range store.Events {
				if event.Stage != "owned" || event.Failed {
					t.Fatal("cleanup attempted an unauthorized record update", event)
				}
			}
			removes := 0
			for _, event := range engine.Events {
				switch event.Method {
				case "CREATE", "IMAGE":
					t.Fatal("cleanup attempted new creation", event)
				case "REMOVE":
					removes++
					if event.Target != id {
						t.Fatal("cleanup removed a different identity", event)
					}
				}
			}
			if removes != tc.removes || len(engine.Resources) != 1-tc.removes {
				t.Fatalf("cleanup removals = %d, surviving resources = %d", removes, len(engine.Resources))
			}
			if tc.removes == 0 && engine.Resources[id] != resource {
				t.Fatal("refused cleanup altered the owned worker")
			}
		})
	}
}
