package domain

import (
	"slices"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestRecoveryKeepsCleanupPendingUntilRemoval(t *testing.T) {
	runner, registry, engine, _, id := registeredSessionFixture(t)
	connection, attachment, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
	if connection != nil {
		defer connection.Close()
	}
	if attachment != nil {
		defer attachment.Close()
	}
	if failure != "" {
		t.Fatal("launch owned worker", failure)
	}
	engine.Exit = 23
	recovery := Recovery{Registry: registry, Engine: engine}

	// Foreground teardown stops the workload before draining its final output.
	// The worker still exists here, so termination alone cannot complete cleanup.
	prepared := recovery.stop(t.Context(), id, "SIGTERM", nil, false)
	if len(prepared.Failures) != 0 || len(prepared.Entries) != 1 {
		t.Fatal("prepare teardown", prepared)
	}
	e := prepared.Entries[0]
	if e.State != "stopping" || e.Resource != "unknown" || e.Cleanup != "pending" ||
		e.WorkloadExit == nil || *e.WorkloadExit != 23 || !slices.Equal(e.Retry, []string{"itos", "container", "stop", id}) {
		t.Fatal("termination reported completed cleanup or lost the workload result/retry", e)
	}
	resource, exists := engine.Resources[worker]
	if !exists || resource.Status != "exited" || resource.Running || engine.Removed {
		t.Fatal("termination did not retain the exited worker for output drain", resource)
	}
	record := registry.Records[id]
	intent, failure := registry.Store.Codec.Decode(registry.Store.Records[record.Intent])
	if failure != "" || record.State != "stopping" || intent.Stage != "owned" || intent.ContainerID != worker {
		t.Fatal("termination lost incomplete cleanup authority", record, intent, failure)
	}
	if registry.Locks[id] || registry.Store.Locked {
		t.Fatal("termination retained the operation lock across output drain")
	}

	completed := recovery.Stop(t.Context(), id)
	if len(completed.Failures) != 0 || len(completed.Entries) != 1 {
		t.Fatal("finish teardown", completed)
	}
	e = completed.Entries[0]
	if e.State != "stopped" || e.Resource != "absent" || e.Cleanup != "complete" ||
		e.WorkloadExit == nil || *e.WorkloadExit != 23 || len(e.Retry) != 0 || e.StopCandidate {
		t.Fatal("removal did not report completed cleanup with the original exit", e)
	}
	if _, exists := engine.Resources[worker]; exists || !engine.Removed {
		t.Fatal("completed cleanup left the worker behind")
	}
	record = registry.Records[id]
	intent, failure = registry.Store.Codec.Decode(registry.Store.Records[record.Intent])
	if failure != "" || record.State != "stopped" || intent.Stage != "absent" || intent.ContainerID != worker {
		t.Fatal("completion lost its concrete absence evidence", record, intent, failure)
	}

	// A late termination phase must preserve completion already won by stop.
	repeated := recovery.stop(t.Context(), id, "SIGTERM", nil, false)
	if len(repeated.Failures) != 0 || len(repeated.Entries) != 1 ||
		repeated.Entries[0].State != "stopped" || repeated.Entries[0].Resource != "absent" ||
		repeated.Entries[0].Cleanup != "complete" || len(repeated.Entries[0].Retry) != 0 {
		t.Fatal("late termination reopened completed cleanup", repeated)
	}
}
