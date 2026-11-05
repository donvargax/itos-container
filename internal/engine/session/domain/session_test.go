package domain

import (
	"context"
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
	if again := s.Cleanup(ctx, created.Reference); again != cleanup {
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
func TestCreateNeedsMatchingCompleteInspection(t *testing.T) {
	for _, condition := range []string{"unavailable", "explicit not-found", "malformed response", "missing State", "different full container ID", "different name", "different image config ID", "different intent label", "different project label", "different fingerprint label", "running state", "exited state after starting", "created state with nonzero PID", "created state with StartedAt"} {
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
	s, d, store, e := serviceFixture()
	store.Fault = "owned-record file fsync"
	created := s.Create(context.Background(), d)
	if created.Outcome != port.OwnershipWrite || created.ID != "" || len(e.Resources) != 1 {
		t.Fatal(created)
	}
	if result := s.Cleanup(context.Background(), created.Reference); result.Outcome != port.Complete {
		t.Fatal(result)
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
