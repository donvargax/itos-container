package domain_test

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestPublicStopRefusesCompletedRecordWithPresentWorker(t *testing.T) {
	s, registry, engine, id := publicRecoveryFixture(t)
	worker := strings.Repeat("2", 64)
	original := engine.Resources[worker]
	r := registry.Records[id]
	exit := 7
	r.State, r.WorkloadExit = "stopped", &exit
	registry.Records[id] = r
	intent := bytes.Clone(registry.Store.Records[r.Intent])
	writes := len(registry.Store.Events)

	result := s.Stop(t.Context(), id)
	requireRecoveryFailure(t, result, id, port.Refused)
	if result.Entries[0].WorkloadExit == nil || *result.Entries[0].WorkloadExit != 7 || !reflect.DeepEqual(registry.Records[id], r) {
		t.Fatal("contradictory completion lost its recorded exit or replaced the tombstone", result)
	}
	if registry.Writes != 0 || registry.Syncs != 0 || len(registry.Store.Events) != writes || !bytes.Equal(registry.Store.Records[r.Intent], intent) || registry.Locks[id] {
		t.Fatal("refused completion changed ownership evidence or retained the lock")
	}
	if !slices.Equal(engine.Events, []string{"open"}) || engine.Removed || !reflect.DeepEqual(engine.Resources, map[string]port.Observation{worker: original}) {
		t.Fatal("completed record authorized mutation of a present worker", engine.Events, engine.Resources)
	}
}

func TestPublicStopRequiresDurableOwnershipBeforeMutation(t *testing.T) {
	for _, stage := range []string{"intent", "owned"} {
		t.Run(stage, func(t *testing.T) {
			s, registry, engine, id := publicRecoveryFixture(t)
			worker := strings.Repeat("2", 64)
			original := engine.Resources[worker]
			r := registry.Records[id]
			i, failure := registry.Store.Codec.Decode(registry.Store.Records[r.Intent])
			if failure != "" {
				t.Fatal(failure)
			}
			if stage == "intent" {
				i.Stage, i.ContainerID = "intent", ""
				r.State = "preparing"
				registry.Records[id] = r
				registry.Store.Records[r.Intent] = registry.Store.Codec.Encode(i)
			}
			intent := bytes.Clone(registry.Store.Records[r.Intent])
			registry.Store.Fault = "owned-record file fsync"

			result := s.Stop(t.Context(), id)
			requireRecoveryFailure(t, result, id, port.IntentWrite)
			retained := registry.Records[id]
			if result.Entries[0].WorkloadExit != nil || retained.State != "failed" || retained.WorkloadExit != nil || retained.LastProblem == nil || *retained.LastProblem != port.IntentWrite || !bytes.Equal(registry.Store.Records[r.Intent], intent) || registry.Locks[id] {
				t.Fatal("failed ownership publication lost uncertainty or the original intent", result, retained)
			}
			if !slices.Equal(engine.Events, []string{"open"}) || engine.Removed || !reflect.DeepEqual(engine.Resources, map[string]port.Observation{worker: original}) {
				t.Fatal("undurable ownership authorized a signal or removal", engine.Events, engine.Resources)
			}

			registry.Store.Fault = ""
			result = s.Stop(t.Context(), id)
			if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || result.Entries[0].WorkloadExit == nil || *result.Entries[0].WorkloadExit != 7 || len(engine.Resources) != 0 {
				t.Fatal("failed ownership publication prevented a verified retry", result)
			}
		})
	}
}

func TestPublicStopRechecksWorkerAfterOwnershipPublication(t *testing.T) {
	for _, resource := range []string{"absent", "running"} {
		t.Run(resource, func(t *testing.T) {
			s, registry, engine, id := publicRecoveryFixture(t)
			worker := strings.Repeat("2", 64)
			running := engine.Resources[worker]
			created := running
			created.Status, created.Running, created.Started, created.PID = "created", false, false, 0
			engine.Resources[worker] = created
			// The first observation permits removal only. The resource changes
			// while ownership durability is established, before the next inspect.
			registry.Store.AfterSave = func() {
				registry.Store.AfterSave = nil
				if resource == "absent" {
					delete(engine.Resources, worker)
				} else {
					engine.Resources[worker] = running
				}
			}

			result := s.Stop(t.Context(), id)
			if !slices.Equal(engine.Events, []string{"open"}) || engine.Removed || registry.Locks[id] {
				t.Fatal("stale created observation authorized a signal or removal", engine.Events)
			}
			r := registry.Records[id]
			i, failure := registry.Store.Codec.Decode(registry.Store.Records[r.Intent])
			if failure != "" || i.ContainerID != worker || r.WorkloadExit != nil {
				t.Fatal("fresh observation lost the owned identity or invented an exit", r, i, failure)
			}
			if resource == "absent" {
				if len(result.Failures) != 0 || len(result.Entries) != 1 {
					t.Fatal("fresh full-ID absence failed cleanup", result)
				}
				e := result.Entries[0]
				if e.State != "stopped" || e.Resource != "absent" || e.Cleanup != "complete" || e.WorkloadExit != nil || e.Retry != nil || e.StopCandidate || r.State != "stopped" || i.Stage != "absent" || len(engine.Resources) != 0 {
					t.Fatal("fresh absence did not become durable completion with unknown exit", result, r, i)
				}
			} else {
				requireRecoveryFailure(t, result, id, port.Incomplete)
				if result.Entries[0].WorkloadExit != nil || r.State != "failed" || r.LastProblem == nil || *r.LastProblem != port.Incomplete || i.Stage != "owned" || !reflect.DeepEqual(engine.Resources, map[string]port.Observation{worker: running}) {
					t.Fatal("newly running worker was removed or recorded as complete", result, r, i)
				}
				result = s.Stop(t.Context(), id)
				if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || len(engine.Resources) != 0 {
					t.Fatal("fresh retry could not terminate the retained running worker", result)
				}
			}
		})
	}
}

func TestPublicStopRevalidatesAuthorityAfterLastResourceInspection(t *testing.T) {
	for _, boundary := range []string{"signal", "remove"} {
		for _, fault := range []string{"record replaced", "daemon changed"} {
			t.Run(boundary+"/"+fault, func(t *testing.T) {
				s, registry, engine, id := publicRecoveryFixture(t)
				worker := strings.Repeat("2", 64)
				original := engine.Resources[worker]
				if boundary == "remove" {
					original.Status, original.Running, original.Started, original.PID = "created", false, false, 0
					engine.Resources[worker] = original
				}
				intent := bytes.Clone(registry.Store.Records[registry.Records[id].Intent])
				var retained port.PublicRecord
				changed := false
				registry.Store.AfterSave = func() {
					registry.Store.AfterSave = nil
					// Change authority after the final resource response has been
					// captured. Its earlier validation must not authorize mutation.
					engine.AfterInspect = func() {
						engine.AfterInspect = nil
						changed = true
						retained = registry.Records[id]
						if fault == "record replaced" {
							problem := port.Internal
							retained.State, retained.LastProblem = "failed", &problem
							registry.Records[id] = retained
						} else {
							engine.DaemonID = "different-daemon"
						}
					}
				}

				result := s.Stop(t.Context(), id)
				requireRecoveryFailure(t, result, id, port.Refused)
				if !changed || result.Entries[0].WorkloadExit != nil || !reflect.DeepEqual(registry.Records[id], retained) || !bytes.Equal(registry.Store.Records[retained.Intent], intent) || registry.Locks[id] {
					t.Fatal("late authority substitution was overwritten or lost its retry", result, registry.Records[id])
				}
				if !slices.Equal(engine.Events, []string{"open"}) || engine.Removed || !reflect.DeepEqual(engine.Resources, map[string]port.Observation{worker: original}) {
					t.Fatal("stale authority reached the barred mutation", engine.Events, engine.Resources)
				}
			})
		}
	}
}

func TestPublicStopSkipsSignalForWorkerExitedDuringOwnershipPublication(t *testing.T) {
	s, registry, engine, id := publicRecoveryFixture(t)
	worker := strings.Repeat("2", 64)
	registry.Store.AfterSave = func() {
		registry.Store.AfterSave = nil
		o := engine.Resources[worker]
		o.Status, o.Running, o.PID = "exited", false, 0
		engine.Resources[worker] = o
	}
	// Inspect observes exit without an explicit workload result. A lost wait
	// reply must neither fabricate that result nor require another signal.
	engine.WaitFailure = port.ExecutionFailed

	result := s.Stop(t.Context(), id)
	if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].State != "stopped" || result.Entries[0].Resource != "absent" || result.Entries[0].Cleanup != "complete" || result.Entries[0].WorkloadExit != nil || result.Entries[0].Retry != nil {
		t.Fatal("exited worker did not complete cleanup with unknown exit", result)
	}
	if !slices.Equal(engine.Events, []string{"open", "wait"}) || !engine.Removed || len(engine.Resources) != 0 || registry.Records[id].State != "stopped" || registry.Records[id].WorkloadExit != nil || registry.Locks[id] {
		t.Fatal("stop signalled an exited worker or lost its completion", engine.Events, registry.Records[id])
	}
}

func TestPublicStopRetainsWorkerStillRunningAfterUncertainKill(t *testing.T) {
	s, registry, engine, id := publicRecoveryFixture(t)
	worker := strings.Repeat("2", 64)
	original := engine.Resources[worker]
	intent := bytes.Clone(registry.Store.Records[registry.Records[id].Intent])
	engine.WaitFailure = port.ExecutionFailed
	engine.After = func(event string) {
		if event == "wait" {
			// Both wait replies are lost and the daemon continues to report
			// the worker running. Sending a signal is not proof of its effect.
			engine.BeforeInspect = func() { engine.Resources[worker] = original }
		}
	}

	result := s.Stop(t.Context(), id)
	requireRecoveryFailure(t, result, id, port.Incomplete)
	r := registry.Records[id]
	if result.Entries[0].WorkloadExit != nil || r.State != "failed" || r.WorkloadExit != nil || r.LastProblem == nil || *r.LastProblem != port.Incomplete || !bytes.Equal(registry.Store.Records[r.Intent], intent) || registry.Locks[id] {
		t.Fatal("uncertain termination invented exit or completion", result, r)
	}
	if !slices.Equal(engine.Events, []string{"open", "SIGTERM", "wait", "SIGKILL", "wait"}) || engine.Removed || !reflect.DeepEqual(engine.Resources, map[string]port.Observation{worker: original}) {
		t.Fatal("uncertain kill skipped escalation or authorized removal", engine.Events, engine.Resources)
	}

	engine.WaitFailure, engine.After, engine.BeforeInspect = "", nil, nil
	result = s.Stop(t.Context(), id)
	if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || result.Entries[0].WorkloadExit == nil || *result.Entries[0].WorkloadExit != 7 || len(engine.Resources) != 0 {
		t.Fatal("uncertain kill prevented a later verified cleanup", result)
	}
}
