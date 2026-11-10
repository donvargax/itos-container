package domain_test

import (
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestRegisteredRunPersistsObservedExitAndActionableCleanup(t *testing.T) {
	for _, tc := range []struct {
		name           string
		exit           int
		cleanupFailure bool
	}{
		{"complete zero", 0, false},
		{"complete nonzero", 7, false},
		{"retry zero", 0, true},
		{"retry nonzero", 7, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, store, engine := f.Fixture()
			registry := &f.Registry{Store: store, Project: b.Frozen.Project}
			runEngine := &f.RunEngine{Engine: engine, Exit: tc.exit}
			if tc.cleanupFailure {
				engine.Following = "still present"
			}
			runner := domain.Runner{Store: store, Registry: registry, Engine: runEngine, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: f.MountWorkspace(b)}
			result := runner.Run(t.Context(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
			if result.SessionID == "" || !result.Exited || result.Exit != tc.exit || registry.Leases[result.SessionID] {
				t.Fatal(result)
			}
			r := registry.Records[result.SessionID]
			if r.WorkloadExit == nil || *r.WorkloadExit != tc.exit {
				t.Fatal("lost observed result", r)
			}
			if tc.cleanupFailure {
				if result.Cleanup == port.Complete || r.State != "failed" {
					t.Fatal(result, r)
				}
			} else if result.Cleanup != port.Complete || r.State != "stopped" {
				t.Fatal(result, r)
			}
			engine.Following = ""
			for range 2 {
				recovery := domain.Recovery{Registry: registry, Engine: runEngine}
				stopped := recovery.Stop(t.Context(), result.SessionID)
				status := recovery.Status(t.Context())
				for _, got := range []port.RecoveryResult{stopped, status} {
					if len(got.Failures) != 0 || len(got.Entries) != 1 {
						t.Fatal("fresh recovery failed", got)
					}
					e := got.Entries[0]
					if e.Cleanup != "complete" || e.Anchor != "absent" || e.WorkloadExit == nil || *e.WorkloadExit != tc.exit || e.Retry != nil {
						t.Fatal("fresh recovery lost completion or observed result", e)
					}
				}
			}
		})
	}
}
