package domain

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

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
