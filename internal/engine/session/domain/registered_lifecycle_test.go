package domain

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func registeredLifecycleFixture() (Runner, approval.Durable, *f.Registry, *f.RunEngine, string) {
	b, store, engine := f.Fixture()
	id, token := strings.Repeat("1", 64), strings.Repeat("2", 64)
	registry := &f.Registry{Store: store, Project: b.Frozen.Project}
	run := &f.RunEngine{Engine: engine, Exit: 17}
	runner := Runner{Store: store, Registry: registry, Engine: run, Workspace: f.MountWorkspace(b),
		Entropy: &f.EntropySequence{Values: []f.Entropy{{Value: id}, {Value: token}}}}
	return runner, approval.Durable{Frozen: b.Frozen}, registry, run, id
}

type delayedRegistration struct {
	port.Registry
	release <-chan struct{}
}

func (r delayedRegistration) Register(ctx context.Context, d approval.Durable, id, token string) (port.Lease, port.Outcome) {
	l, f := r.Registry.Register(ctx, d, id, token)
	<-r.release
	return l, f
}

func TestRegisteredRunReleasesLateRegistrationWithoutLaunching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, frozen, registry, engine, id := registeredLifecycleFixture()
		release := make(chan struct{})
		runner.Registry = delayedRegistration{registry, release}
		start := time.Now()
		result := runner.Run(context.Background(), frozen, []string{"probe"}, port.Streams{})
		if result.Outcome != port.CallTimeout || result.Operation != "session registration" || result.Started || result.Exited || result.SessionID != "" || time.Since(start) != 10*time.Second {
			t.Error("pending registration lost its bounded startup result", result, time.Since(start))
		}
		synctest.Wait()
		if !registry.Leases[id] || registry.Records[id].State != "preparing" {
			t.Error("accepted registration was forgotten before its response")
		}
		close(release)
		synctest.Wait()
		if registry.Leases[id] || registry.Locks[id] || len(engine.Events) != 0 {
			t.Fatal("late registration retained its lease or launched work", engine.Events)
		}
		recovered := (Recovery{Registry: registry, Engine: engine}).Stop(context.Background(), id)
		if len(recovered.Failures) != 0 || len(recovered.Entries) != 1 || recovered.Entries[0].Cleanup != "complete" || recovered.Entries[0].WorkloadExit != nil || len(engine.Events) != 0 {
			t.Fatal("fresh recovery could not cancel the original uncreated session", recovered)
		}
	})
}

func TestRegisteredRunClassifiesRegistrationRefusalBeforeLaunch(t *testing.T) {
	for _, tc := range []struct{ fault, want port.Outcome }{
		{port.Refused, port.RegistrationOwnership},
		{port.Incomplete, port.RegistrationTemporary},
	} {
		t.Run(string(tc.fault), func(t *testing.T) {
			runner, frozen, registry, engine, _ := registeredLifecycleFixture()
			registry.Failure = tc.fault
			result := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{})
			if !reflect.DeepEqual(result, port.RunResult{Outcome: tc.want}) || len(registry.Records) != 0 || len(registry.Leases) != 0 || len(engine.Events) != 0 || registry.Store.Locked {
				t.Fatal("registration refusal published a session or reached the engine", result)
			}
			ref := port.Reference(frozen.Frozen.Project, strings.Repeat("2", 64))
			intent, failure := registry.Store.Codec.Decode(registry.Store.Records[ref])
			if failure != "" || intent.Stage != "reserved" || intent.ContainerID != "" {
				t.Fatal("registration refusal lost the original no-create evidence", intent, failure)
			}
		})
	}
}

// Model a publication that is visible but whose durability acknowledgement
// failed. The in-memory registry still owns validation and stored records.
type exitPublicationRegistry struct{ port.Registry }
type exitPublicationSession struct{ port.Session }

func (r exitPublicationRegistry) Open(ctx context.Context, id string, ro bool) (port.Session, port.Outcome) {
	s, f := r.Registry.Open(ctx, id, ro)
	if s == nil {
		return nil, f
	}
	return exitPublicationSession{s}, f
}
func (s exitPublicationSession) Save(r port.PublicRecord) port.Outcome {
	firstExit := s.Record().WorkloadExit == nil && r.WorkloadExit != nil
	if f := s.Session.Save(r); f != "" {
		return f
	}
	if firstExit {
		return port.IntentWrite
	}
	return ""
}

func TestRegisteredRunPreservesExitWhenTeardownPublicationIsUncertain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, frozen, registry, engine, id := registeredLifecycleFixture()
		runner.Registry = exitPublicationRegistry{registry}
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		engine.After = func(event string) {
			if event == "wait" && ctx.Err() == nil {
				cancel(port.Interrupt{Signal: "SIGTERM"})
			}
		}
		result := runner.Run(ctx, frozen, []string{"probe"}, port.Streams{})
		if !result.Exited || result.Exit != 17 || result.Signal != "SIGTERM" || result.Outcome != "" || result.Cleanup != port.IntentWrite || result.Stream != "" {
			t.Fatal("uncertain teardown publication hid the observed workload exit", result)
		}
		r := registry.Records[id]
		if r.WorkloadExit == nil || *r.WorkloadExit != 17 || r.State != "failed" || r.LastProblem == nil || *r.LastProblem != port.IntentWrite || registry.Leases[id] || registry.Locks[id] {
			t.Fatal("uncertain publication lost retry evidence or retained authority", r)
		}
		if engine.Removed || engine.Resources[engine.CreateID].Status != "exited" || slices.Contains(engine.Events, "SIGKILL") {
			t.Fatal("uncertain publication claimed cleanup or signalled an exited worker", engine.Events)
		}
	})
}

func TestRegisteredRunReportsStreamDiagnosticPublicationFailure(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			runner, frozen, registry, engine, id := registeredLifecycleFixture()
			engine.DrainUntilCancel = true
			registry.BeforeSave = func(r port.PublicRecord) {
				if failSave && r.LastProblem != nil && *r.LastProblem == port.StreamTimeout {
					registry.SaveFailure = port.IntentWrite
				}
			}
			start := time.Now()
			result := runner.Run(context.Background(), frozen, []string{"probe"}, port.Streams{})
			want := port.Complete
			if failSave {
				want = port.IntentWrite
			}
			if result.Cleanup != want || result.Stream != port.StreamTimeout || !result.Exited || result.Exit != 17 || result.Outcome != "" || time.Since(start) != 10*time.Second {
				t.Fatal("diagnostic publication hid a failure or the workload exit", result)
			}
			r := registry.Records[id]
			if r.State != "stopped" || r.WorkloadExit == nil || *r.WorkloadExit != 17 || (r.LastProblem == nil) != failSave || len(engine.Resources) != 0 || registry.Leases[id] || registry.Locks[id] {
				t.Fatal("diagnostic publication changed completed cleanup or retained authority", r)
			}
			if !failSave && *r.LastProblem != port.StreamTimeout {
				t.Fatal("healthy storage lost the stream timeout", r)
			}
		})
	}
}
