package domain_test

import (
	"strings"
	"testing"

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
