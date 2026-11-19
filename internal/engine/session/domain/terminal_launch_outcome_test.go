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
					// A retained home whose root another party opened up is
					// refused once the worker exists, before it can start.
					r := x.engine.Roots[g.Name]
					r.Mode = 0777
					x.engine.Roots[g.Name] = r
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

func TestLaunchOutcomeJoinsLaunchDecisionWithClientLoss(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failure   port.Outcome
		loss      port.Outcome
		cancelled bool
		want      port.Outcome
	}{
		{"no loss keeps no outcome", "", "", false, ""},
		{"no loss never invents an outcome from cancellation", "", "", true, ""},
		{"loss after a launched worker", "", port.TerminalDisconnected, false, port.TerminalDisconnected},
		{"loss that cancelled a launched worker", "", port.TerminalDisconnected, true, port.TerminalDisconnected},
		{"loss only found by the final poll keeps a refusal", port.HomeRefused, port.TerminalDisconnected, false, port.HomeRefused},
		{"loss only found by the final poll keeps a credential refusal", port.CredentialRefused, port.TerminalUnavailable, false, port.CredentialRefused},
		{"loss only found by the final poll keeps an unavailable engine", port.Incomplete, port.TerminalDisconnected, false, port.Incomplete},
		{"loss that cancelled launch explains its failure", port.Incomplete, port.TerminalDisconnected, true, port.TerminalDisconnected},
		{"loss that cancelled launch replaces an unavailable home", port.HomeUnavailable, port.TerminalUnavailable, true, port.TerminalUnavailable},
		{"workspace refusal stands", port.Refused, port.TerminalDisconnected, false, port.Refused},
		{"workspace refusal stands over a cancelling loss", port.Refused, port.TerminalDisconnected, true, port.Refused},
		{"unverified isolation stands", port.IsolationUnverified, port.TerminalDisconnected, false, port.IsolationUnverified},
		{"unverified isolation stands over a cancelling loss", port.IsolationUnverified, port.TerminalDisconnected, true, port.IsolationUnverified},
		{"mount refusal stands", port.MountRefused, port.TerminalDisconnected, false, port.MountRefused},
		{"mount refusal stands over a cancelling loss", port.MountRefused, port.TerminalDisconnected, true, port.MountRefused},
		{"superseding stop stands", launchSuperseded, port.TerminalDisconnected, false, launchSuperseded},
		{"superseding stop stands over a cancelling loss", launchSuperseded, port.TerminalDisconnected, true, launchSuperseded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := launchOutcome(tc.failure, tc.loss, tc.cancelled); got != tc.want {
				t.Fatalf("launchOutcome(%q, %q, %v) = %q, want %q", tc.failure, tc.loss, tc.cancelled, got, tc.want)
			}
		})
	}
}

// The client's loss can end the very observation of the mount detector. The
// refusal that cancellation produces says nothing about the checkout, so the
// run reports the loss and the started worker keeps the owned grace period.
func TestHangupCancellingTheMountDetectorReportsTheLoss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, frozen, registry, engine, id := registeredLifecycleFixture()
		provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
		lease, err := provider.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()

		started, lost := false, false
		engine.AfterStart = func() { started = true }
		engine.BeforeInspect = func() {
			if !started || lost {
				return
			}
			// The detector's first observation finds launch already cancelled.
			lost = true
			provider.Hangup(false)
			<-lease.Done()
			synctest.Wait()
		}
		engine.Exit, engine.WaitUntilCancel = 17, true
		got := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{Terminal: lease})
		if !lost {
			t.Fatal("the detector never observed the worker")
		}
		if got.Outcome != port.TerminalDisconnected || got.MountDiagnostic != (port.MountDiagnostic{}) {
			t.Errorf("an interrupted detector reported a mount refusal: %+v", got)
		}
		if !got.Exited || got.Exit != 17 || got.Cleanup != port.Complete ||
			!slices.Contains(engine.Events, "SIGTERM") || slices.Contains(engine.Events, "SIGKILL") {
			t.Errorf("the started worker lost its owned grace period: %+v %v", got, engine.Events)
		}
		if registry.Records[id].MountDetector != "" || len(engine.Resources) != 0 || len(engine.Anchors) != 0 {
			t.Errorf("an unfinished detector left a verdict or an owned resource: %q %v", registry.Records[id].MountDetector, engine.Events)
		}
	})
}
