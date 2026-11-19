package domain

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	tf "github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
)

func TestLateHangupPreservesMountRefusal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, frozen, registry, engine, _ := registeredLifecycleFixture()
		provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}, ObservationDelay: time.Hour}
		lease, err := provider.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()

		workspace := runner.Workspace.(*f.Workspace)
		mount := workspace.Mounts[123]
		mount.Inode = "different"
		workspace.Mounts[123] = mount
		registry.BeforeSave = func(record port.PublicRecord) {
			if record.MountDetector == port.DetectorRefused {
				provider.Hangup(false)
			}
		}
		engine.IgnoreSignal, engine.WaitUntilCancel = true, true
		start := time.Now()
		got := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{Terminal: lease})
		if got.Outcome != port.MountRefused || got.MountDiagnostic.Check != "mounted-identity" {
			t.Errorf("late hangup replaced the mount refusal: %+v", got)
		}
		if time.Since(start) >= budget.GraceDuration || slices.Contains(engine.Events, "SIGTERM") || !slices.Contains(engine.Events, "SIGKILL") ||
			got.Cleanup != port.Complete || len(engine.Resources) != 0 || len(engine.Anchors) != 0 {
			t.Errorf("refused worker did not receive immediate owned cleanup: %v", engine.Events)
		}
	})
}

func TestLateHangupLeavesSupersedingStopInControl(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, frozen, registry, engine, id := registeredLifecycleFixture()
		provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}, ObservationDelay: time.Hour}
		lease, err := provider.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()

		// Stop takes the transition between create and start. The final launch
		// poll will see the hangup while that stop still owns cleanup.
		registry.AfterClose = func(id string) {
			record := registry.Records[id]
			if record.State == "ready" {
				record.State = "stopping"
				registry.Records[id] = record
				provider.Hangup(false)
			}
		}
		got := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{Terminal: lease})
		if got.Outcome != "" || got.Cleanup != port.Incomplete || got.Started || got.Exited {
			t.Errorf("run lost the superseding stop's pending result: %+v", got)
		}
		if registry.Records[id].State != "stopping" || engine.Resources[engine.CreateID].Status != "created" || len(engine.Anchors) != 1 {
			t.Errorf("run took cleanup from the superseding stop: %v", engine.Events)
		}
	})
}

// A refusal launch reached on its own stands even when the client hangs up
// while launch is still releasing its entry. Only the final poll sees the
// hangup, so no cancellation caused the refusal.
func TestLateHangupPreservesHomeAndCredentialRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		want port.Outcome
	}{
		{"home", port.HomeRefused},
		{"credential", port.CredentialRefused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var runner Runner
				var frozen approval.Durable
				var registry *f.Registry
				var engine *f.RunEngine
				switch tc.name {
				case "home":
					x := domainHomeFixture()
					g := x.seed()
					o := x.engine.Homes[g.Name]
					o.Created = "replacement"
					x.engine.Homes[g.Name] = o
					runner, frozen, registry, engine = x.runner, x.frozen, x.registry, x.engine.RunEngine
				case "credential":
					runner, frozen, registry, engine, _ = registeredLifecycleFixture()
					// A staged kind with no stager can never be satisfied.
					registry.Store.Codec.(*f.Codec).Value.StagingKinds = f.MemoryCredentialFixture(map[string][]byte{"claude-token": []byte("synthetic\n")}).Sources.Kinds()
				}
				provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}, ObservationDelay: time.Hour}
				lease, err := provider.Acquire(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
				lost := false
				registry.AfterClose = func(string) {
					if !lost {
						lost = true
						provider.Hangup(false)
					}
				}
				got := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{Terminal: lease})
				if !lost {
					t.Fatal("the client was never lost")
				}
				if got.Outcome != tc.want || got.Started || got.Exited || got.Cleanup != port.Complete {
					t.Errorf("late hangup replaced the launch refusal: %+v", got)
				}
				if slices.Contains(engine.Events, "start") || len(engine.Resources) != 0 || len(engine.Anchors) != 0 {
					t.Errorf("refused launch kept owned resources: %v", engine.Events)
				}
			})
		})
	}
}
