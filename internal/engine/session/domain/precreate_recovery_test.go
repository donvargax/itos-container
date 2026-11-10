package domain_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestPrecreateRecoveryRetainsProofAcrossPersistenceFailures(t *testing.T) {
	for _, fault := range []string{"", "cancelled-record file fsync", "cancelled-record parent fsync", "public write", "invalid exit", "changed authority", "create may have been sent"} {
		t.Run(fault, func(t *testing.T) {
			b, store, engine := f.Fixture()
			id, token := strings.Repeat("a", 64), strings.Repeat("1", 64)
			i, failure := store.Prepare(t.Context(), ap.Durable{Frozen: b.Frozen}, token)
			if failure != "" {
				t.Fatal(failure)
			}
			if fault == "create may have been sent" {
				r := i.Record()
				r.Stage = "intent"
				if failure = i.Save(r); failure != "" {
					t.Fatal(failure)
				}
			}
			i.Close()
			registry := &f.Registry{Store: store, Project: b.Frozen.Project}
			lease, failure := registry.Register(t.Context(), ap.Durable{Frozen: b.Frozen}, id, token)
			if failure != "" {
				t.Fatal(failure)
			}
			defer lease.Close()
			store.Fault = fault
			if fault == "public write" {
				registry.SaveFailure = port.IntentWrite
			}
			if fault == "invalid exit" {
				r := registry.Records[id]
				exit := 0
				r.WorkloadExit = &exit
				registry.Records[id] = r
			}
			if fault == "changed authority" {
				registry.AfterOpen = func(string) { store.UnsafeSnapshot = true }
			}
			runEngine := &f.RunEngine{Engine: engine}
			recovery := domain.Recovery{Registry: registry, Engine: runEngine}
			result := recovery.Stop(t.Context(), id)
			if fault != "" {
				if len(result.Failures) == 0 || result.Entries[0].Cleanup == "complete" {
					t.Fatal("failed cancellation released authority", result)
				}
				if fault == "create may have been sent" || fault == "invalid exit" || fault == "changed authority" {
					return
				}
				store.Fault, registry.SaveFailure = "", ""
				result = recovery.Stop(t.Context(), id)
			}
			if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || result.Entries[0].WorkloadExit != nil {
				t.Fatal("pre-create cancellation failed", result)
			}
			if result = recovery.Stop(t.Context(), id); len(result.Failures) != 0 {
				t.Fatal("repeated cancellation", result)
			}
			writes := len(store.Events)
			result = recovery.Status(t.Context())
			if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" || result.Entries[0].WorkloadExit != nil || len(store.Events) != writes || len(runEngine.Events) != 0 || len(engine.Events) != 0 {
				t.Fatal("cancelled status wrote state, contacted engine or invented a result", result)
			}
		})
	}
}

func TestRegisteredRunPreservesPrecreateStopResult(t *testing.T) {
	for _, fault := range []string{"", "cancelled-record file fsync", "cancelled-record parent fsync", "status unavailable"} {
		t.Run(fault, func(t *testing.T) {
			b, store, engine := f.Fixture()
			id, token := strings.Repeat("a", 64), strings.Repeat("1", 64)
			ref := port.Reference(b.Frozen.Project, token)
			registry := &f.Registry{Store: store, Project: b.Frozen.Project}
			runEngine := &f.RunEngine{Engine: engine}
			var stoppedRecord port.PublicRecord
			var stoppedIntent string
			var writes, syncs, intentEvents int
			registry.AfterRegister = func() {
				if fault != "status unavailable" {
					store.Fault = fault
				}
				stopped := (domain.Recovery{Registry: registry, Engine: runEngine}).Stop(t.Context(), id)
				if fault == "" || fault == "status unavailable" {
					if len(stopped.Failures) != 0 || len(stopped.Entries) != 1 || stopped.Entries[0].Cleanup != "complete" {
						t.Fatal("stop did not complete pre-create cancellation", stopped)
					}
				} else if len(stopped.Failures) == 0 || stopped.Entries[0].Cleanup != "unknown" {
					t.Fatal("failed cancellation reported complete", stopped)
				}
				store.Fault = ""
				if fault == "status unavailable" {
					registry.ControllerFailure = port.Incomplete
				}
				stoppedRecord, stoppedIntent = registry.Records[id], string(store.Records[ref])
				writes, syncs, intentEvents = registry.Writes, registry.Syncs, len(store.Events)
			}
			runner := domain.Runner{Store: store, Registry: registry, Engine: runEngine,
				Entropy: &f.EntropySequence{Values: []f.Entropy{{Value: id}, {Value: token}}}, Workspace: f.MountWorkspace(b)}
			started := time.Now()
			result := runner.Run(t.Context(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
			if result.Deadline.Before(started.Add(30*time.Second)) || result.Deadline.After(time.Now().Add(30*time.Second)) {
				t.Fatal("resumed run lost its thirty-second teardown deadline", result.Deadline)
			}
			want := port.RunResult{SessionID: id, Reference: ref, Cleanup: port.Incomplete, Deadline: result.Deadline}
			if fault == "" {
				want.Cleanup, want.Outcome = port.Complete, port.Incomplete
			}
			if result != want {
				t.Fatalf("resumed run lost stop's cleanup result or invented a workload exit: got %#v, want %#v", result, want)
			}
			if !reflect.DeepEqual(registry.Records[id], stoppedRecord) || string(store.Records[ref]) != stoppedIntent ||
				registry.Writes != writes || registry.Syncs != syncs || len(store.Events) != intentEvents {
				t.Fatal("resumed run rewrote stop's durable evidence")
			}
			if len(engine.Events) != 0 || len(runEngine.Events) != 0 || len(engine.Resources) != 0 {
				t.Fatal("resumed run contacted the engine after pre-create stop")
			}
			if registry.Leases[id] || registry.Locks[id] || registry.Readers[id] != 0 || store.Locked {
				t.Fatal("resumed run retained a lease or operation lock")
			}
		})
	}
}
