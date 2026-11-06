package domain_test

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func requireRecoveryFailure(t *testing.T, result port.RecoveryResult, id string, want port.Outcome) {
	t.Helper()
	if !reflect.DeepEqual(result.Failures, []port.RecoveryFailure{{Session: id, Outcome: want}}) || len(result.Entries) != 1 {
		t.Fatal("recovery lost the session or failure cause", result)
	}
	e := result.Entries[0]
	if e.ID != id || e.State != "failed" || e.Resource != "unknown" || e.Cleanup != "unknown" || e.Controller != "absent" || !e.StopCandidate || !slices.Equal(e.Retry, []string{"itos", "container", "stop", id}) {
		t.Fatal("failed recovery claimed completion or lost the retry", e)
	}
}

func TestPublicStatusRejectsInvalidNamesAndContinuesSafeInspection(t *testing.T) {
	s, registry, engine, id := publicRecoveryFixture(t)
	original := registry.Records[id]
	for _, invalid := range []string{"../outside", strings.Repeat("z", 64)} {
		r := original
		r.ID = invalid
		registry.Records[invalid] = r
	}
	before := len(registry.Store.Events)
	result := s.Status(t.Context())
	if !reflect.DeepEqual(result.Failures, []port.RecoveryFailure{{Outcome: port.Invalid}, {Outcome: port.Invalid}}) || len(result.Entries) != 1 {
		t.Fatal("invalid names were trusted or hid the valid session", result)
	}
	e := result.Entries[0]
	if e.ID != id || e.State != "running" || e.Resource != "running" || e.Cleanup != "pending" || e.WorkloadExit != nil || !e.StopCandidate || !slices.Equal(e.Retry, []string{"itos", "container", "stop", id}) {
		t.Fatal("safe session lost its independent inspection", e)
	}
	if !reflect.DeepEqual(registry.Readers, map[string]int{id: 0}) || !slices.Equal(engine.Events, []string{"open"}) {
		t.Fatal("status opened an invalid lookup or retained a reader", registry.Readers, engine.Events)
	}
	if registry.Records[id] != original || registry.Writes != 0 || registry.Syncs != 0 || len(registry.Store.Events) != before {
		t.Fatal("partial status changed protected evidence")
	}
}

func TestPublicStatusRequiresIntentAndResourceAbsenceForCompletion(t *testing.T) {
	for _, resourcePresent := range []bool{true, false} {
		t.Run(map[bool]string{true: "worker remains", false: "intent not completed"}[resourcePresent], func(t *testing.T) {
			s, registry, engine, id := publicRecoveryFixture(t)
			r := registry.Records[id]
			exit := 7
			r.State, r.WorkloadExit = "stopped", &exit
			registry.Records[id] = r
			if !resourcePresent {
				delete(engine.Resources, strings.Repeat("2", 64))
			}
			before := len(registry.Store.Events)
			result := s.Status(t.Context())
			requireRecoveryFailure(t, result, id, port.Incomplete)
			if result.Entries[0].WorkloadExit == nil || *result.Entries[0].WorkloadExit != 7 {
				t.Fatal("contradictory cleanup erased the observed workload exit", result)
			}
			if registry.Records[id] != r || registry.Writes != 0 || registry.Syncs != 0 || len(registry.Store.Events) != before || !slices.Equal(engine.Events, []string{"open"}) {
				t.Fatal("status repaired contradictory completion")
			}
		})
	}
}

func TestPublicRecoveryRejectsAuthorityChangedAfterEngineOpen(t *testing.T) {
	for _, operation := range []string{"status", "stop"} {
		for _, fault := range []string{"record replaced", "unsafe intent"} {
			t.Run(operation+"/"+fault, func(t *testing.T) {
				s, registry, engine, id := publicRecoveryFixture(t)
				original := registry.Records[id]
				replacement := original
				replacement.State = "failed"
				before := len(registry.Store.Events)
				want := port.Refused
				if fault == "unsafe intent" {
					want = port.Unsafe
				}
				engine.AfterOpen = func() {
					if fault == "record replaced" {
						registry.Records[id] = replacement
					} else {
						registry.Store.UnsafeRecord = true
					}
				}
				var result port.RecoveryResult
				if operation == "status" {
					result = s.Status(t.Context())
				} else {
					result = s.Stop(t.Context(), id)
				}
				requireRecoveryFailure(t, result, id, want)
				if result.Entries[0].WorkloadExit != nil || registry.Writes != 0 || registry.Syncs != 0 || len(registry.Store.Events) != before || registry.Locks[id] || registry.Readers[id] != 0 {
					t.Fatal("replaced authority acquired progress or retained its lock", result)
				}
				if !reflect.DeepEqual(engine.Engine.Events, []f.EngineEvent{{Method: "INFO"}}) || !slices.Equal(engine.Events, []string{"open"}) {
					t.Fatal("changed storage authority reached resource inspection or mutation", engine.Engine.Events, engine.Events)
				}
				if fault == "record replaced" && registry.Records[id] != replacement {
					t.Fatal("recovery overwrote the replacement record")
				}
			})
		}
	}
}

func TestPublicIntentOnlyRecoveryRejectsUntrustedNameObservation(t *testing.T) {
	for _, operation := range []string{"status", "stop"} {
		for _, fault := range []string{"changed engine", "foreign token"} {
			t.Run(operation+"/"+fault, func(t *testing.T) {
				s, registry, engine, id := publicRecoveryFixture(t)
				r := registry.Records[id]
				i, failure := registry.Store.Codec.Decode(registry.Store.Records[r.Intent])
				if failure != "" {
					t.Fatal(failure)
				}
				i.Stage, i.ContainerID = "intent", ""
				data := registry.Store.Codec.Encode(i)
				registry.Store.Records[r.Intent] = bytes.Clone(data)
				before := len(registry.Store.Events)
				worker := strings.Repeat("2", 64)
				original := engine.Resources[worker]
				if fault == "changed engine" {
					engine.BeforeInspect = func() { engine.Replaced = true }
				} else {
					engine.Inspection = "different intent label"
					// The full-ID observation would match. An untrusted name
					// response must not supply authority to continue to it.
					engine.AfterInspect = func() { engine.Inspection = "" }
				}
				var result port.RecoveryResult
				if operation == "status" {
					result = s.Status(t.Context())
				} else {
					result = s.Stop(t.Context(), id)
				}
				requireRecoveryFailure(t, result, id, port.Refused)
				if result.Entries[0].WorkloadExit != nil || registry.Records[id] != r || registry.Writes != 0 || registry.Syncs != 0 || len(registry.Store.Events) != before || !bytes.Equal(registry.Store.Records[r.Intent], data) {
					t.Fatal("untrusted name observation acquired ownership or progress")
				}
				if len(engine.Engine.Events) != 3 || engine.Engine.Events[2].Method != "INSPECT" || engine.Engine.Events[2].Target != i.Name || !slices.Equal(engine.Events, []string{"open"}) || !reflect.DeepEqual(engine.Resources, map[string]port.Observation{worker: original}) {
					t.Fatal("untrusted name observation authorized another engine operation", engine.Engine.Events, engine.Events)
				}
			})
		}
	}
}

func TestPublicStopRejectsInvalidIDBeforeRegistryAccess(t *testing.T) {
	for _, id := range []string{"", "../outside", strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		t.Run(id, func(t *testing.T) {
			s := domain.Recovery{Registry: &f.Registry{Failure: port.Internal}}
			result := s.Stop(t.Context(), id)
			if len(result.Entries) != 0 || !reflect.DeepEqual(result.Failures, []port.RecoveryFailure{{Outcome: port.Invalid}}) {
				t.Fatal("invalid ID reached registry access or was returned as trusted identity", result)
			}
		})
	}
}

func TestPublicStopReportsFailedUncertaintyPublicationAndCanRetry(t *testing.T) {
	s, registry, engine, id := publicRecoveryFixture(t)
	original := registry.Records[id]
	exit := 7
	original.WorkloadExit = &exit
	registry.Records[id] = original
	registry.SaveFailure, engine.Unavailable = port.IntentWrite, true
	before := len(registry.Store.Events)
	result := s.Stop(t.Context(), id)
	requireRecoveryFailure(t, result, id, port.IntentWrite)
	if result.Entries[0].WorkloadExit == nil || *result.Entries[0].WorkloadExit != 7 || registry.Records[id] != original || registry.Writes != 0 || registry.Syncs != 0 || len(registry.Store.Events) != before || registry.Locks[id] {
		t.Fatal("failed uncertainty publication changed evidence or lost the observed exit", result)
	}
	if !reflect.DeepEqual(engine.Engine.Events, []f.EngineEvent{{Method: "INFO"}}) || len(engine.Events) != 0 {
		t.Fatal("unavailable engine authorized mutation", engine.Engine.Events, engine.Events)
	}
	registry.SaveFailure, engine.Unavailable = "", false
	result = s.Stop(t.Context(), id)
	if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || result.Entries[0].WorkloadExit == nil || *result.Entries[0].WorkloadExit != 7 || len(engine.Resources) != 0 {
		t.Fatal("failed publication prevented owned retry or changed its result", result)
	}
}

func TestRegisteredRunRetainsObservedExitWhenPublicationFails(t *testing.T) {
	b, store, engine := f.Fixture()
	registry := &f.Registry{Store: store, Project: b.Frozen.Project}
	runEngine := &f.RunEngine{Engine: engine, Exit: 7}
	runEngine.After = func(event string) {
		if event == "drain" {
			registry.SaveFailure = port.IntentWrite
		}
	}
	runner := domain.Runner{Store: store, Registry: registry, Engine: runEngine, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
	result := runner.Run(t.Context(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
	if result.SessionID == "" || !result.Exited || result.Exit != 7 || result.Outcome != "" || result.Cleanup != port.IntentWrite {
		t.Fatal("failed exit publication erased the observed result or claimed cleanup", result)
	}
	r := registry.Records[result.SessionID]
	if r.State != "running" || r.WorkloadExit != nil || r.LastProblem != nil || registry.Leases[result.SessionID] || registry.Locks[result.SessionID] || len(engine.Resources) != 1 {
		t.Fatal("failed exit publication advanced cleanup or retained a lock", r)
	}
	for _, event := range engine.Events {
		if event.Method == "REMOVE" {
			t.Fatal("cleanup removed the worker before exit publication")
		}
	}
	registry.SaveFailure = ""
	retried := (domain.Recovery{Registry: registry, Engine: runEngine}).Stop(t.Context(), result.SessionID)
	if len(retried.Failures) != 0 || len(retried.Entries) != 1 || retried.Entries[0].Cleanup != "complete" || retried.Entries[0].WorkloadExit != nil || len(engine.Resources) != 0 {
		t.Fatal("retry lost ownership or invented the unpublished exit", retried)
	}
}
