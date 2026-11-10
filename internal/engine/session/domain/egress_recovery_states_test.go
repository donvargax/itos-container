package domain

import (
	"reflect"
	"slices"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestEgressRecoveryKeepsUnexpectedProxyStatesRetryable(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*port.Observation)
	}{
		{"dead", func(o *port.Observation) { o.Status = "dead" }},
		{"removing", func(o *port.Observation) { o.Status = "removing" }},
		{"paused", func(o *port.Observation) { o.Status, o.Running = "paused", true }},
		{"restarting", func(o *port.Observation) { o.Status = "restarting" }},
		{"missing state", func(o *port.Observation) { o.StatePresent = false }},
		{"running without flag", func(o *port.Observation) { o.Status = "running" }},
		{"created after start", func(o *port.Observation) { o.Status, o.Started = "created", true }},
		{"created with PID", func(o *port.Observation) { o.Status, o.PID = "created", 123 }},
		{"exited with PID", func(o *port.Observation) { o.PID = 123 }},
		{"exited still running", func(o *port.Observation) { o.Running = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, registry, engine, id, r := seedEgressSession(t, 9)
			o := engine.Resources[r.Egress.Proxy.ID]
			o.Status, o.Running, o.Started, o.PID = "exited", false, false, 0
			tc.edit(&o)
			engine.Resources[o.ID] = o
			recovery := Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}
			before := registry.Writes
			status := recovery.Status(t.Context())
			if len(status.Failures) != 1 || status.Failures[0].Outcome != port.Incomplete || registry.Writes != before {
				t.Fatal("unexpected owned state must be incomplete without a status write", status)
			}
			for range 2 {
				result := recovery.Stop(t.Context(), id)
				if len(result.Failures) != 1 || result.Failures[0].Outcome != port.Incomplete || len(result.Entries) != 1 || !reflect.DeepEqual(result.Entries[0].Retry, []string{"itos", "container", "stop", id}) {
					t.Fatal("owned proxy state lost its retry", result)
				}
				saved := registry.Records[id]
				if saved.State != "failed" || saved.LastProblem == nil || *saved.LastProblem != port.Incomplete {
					t.Fatal("retryable proxy state was not recorded", saved)
				}
				if len(engine.Resources) != 1 || len(engine.Networks) != 2 || len(engine.Anchors) != 1 || slices.Contains(engine.Events, "proxy remove") {
					t.Fatal("uncertain proxy state released resources", engine.Events)
				}
			}
			// A later, confirmed stopped observation permits the retained copy and
			// ordinary ordered teardown. No restart or replacement is needed.
			o.StatePresent, o.Status, o.Running, o.PID = true, "exited", false, 0
			engine.Resources[o.ID] = o
			result := recovery.Stop(t.Context(), id)
			if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || len(result.Entries[0].Retry) != 0 || len(engine.Resources) != 0 || len(engine.Networks) != 0 || len(engine.Anchors) != 0 || !slices.Contains(engine.Events, "proxy remove") {
				t.Fatal("stopped owned proxy did not recover", result, engine.Events)
			}
		})
	}
}
