package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"strings"
	"testing"
)

func TestEgressStopRevalidatesEachDestructiveBoundary(t *testing.T) {
	for _, tc := range []struct {
		fault string
		want  port.Outcome
	}{
		{"before signal", port.Unsafe}, {"proxy signal", port.ExecutionFailed}, {"proxy wait", port.Incomplete}, {"after wait", port.Refused},
		{"after copy authority", port.Unsafe}, {"after copy identity", port.Refused}, {"after removal pin", port.Refused}, {"after absence authority", port.Unsafe}, {"absence write", port.CompletionWrite},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			runner, registry, engine, id, r := seedEgressSession(t, 9)
			store := registry.Store
			engine.EgressFault = tc.fault
			engine.After = func(event string) {
				switch {
				case event == "proxy copy" && tc.fault == "after copy authority":
					store.UnsafeRecord = true
				case event == "proxy copy" && tc.fault == "after copy identity":
					engine.Inspection = "different name"
				case event == "proxy remove" && tc.fault == "after removal pin":
					engine.Replaced = true
				case event == "proxy remove" && tc.fault == "absence write":
					store.Fault = "absent-record write"
				}
			}
			engine.AfterInspect = func() {
				last := engine.Engine.Events[len(engine.Engine.Events)-1]
				if last.Target != r.Egress.Proxy.ID {
					return
				}
				if tc.fault == "before signal" {
					store.UnsafeRecord = true
				}
				if tc.fault == "after absence authority" && last.Status == port.NotFound {
					store.UnsafeRecord = true
				}
			}
			if tc.fault == "after wait" {
				engine.AfterProxyWait = func() { engine.Inspection = "different name" }
			}
			result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
			if len(result.Failures) != 1 || result.Failures[0].Outcome != tc.want || len(engine.Networks) != 2 || len(engine.Anchors) != 1 {
				t.Errorf("%s = %#v", tc.fault, result)
			}
		})
	}
}
func TestEgressLostProxyAdoptionMustBeDurable(t *testing.T) {
	for _, fault := range []string{"authority", "save", "substitution", "healthy"} {
		t.Run(fault, func(t *testing.T) {
			runner, registry, engine, id, r := seedEgressSession(t, 5)
			store := registry.Store
			b := store.Codec.(*f.Codec).Value
			o := f.EgressProxy(r, b)
			o.ID = strings.Repeat("e", 64)
			engine.Resources[o.ID] = o
			engine.ProxyID = o.ID
			engine.AfterInspect = func() {
				last := engine.Engine.Events[len(engine.Engine.Events)-1]
				if last.Target == r.Egress.Proxy.Name {
					if fault == "authority" {
						store.UnsafeRecord = true
					}
					if fault == "save" {
						store.Fault = "cancelled-record write"
					}
				}
			}
			store.AfterSave = func() {
				current, _ := store.Codec.Decode(store.Records[registry.Records[id].Intent])
				if fault == "substitution" && current.Egress.Proxy.Stage == "owned" {
					engine.Inspection = "different name"
				}
			}
			result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
			want := map[string]port.Outcome{"authority": port.Unsafe, "save": port.OwnershipWrite, "substitution": port.Refused}[fault]
			if fault == "healthy" {
				if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" {
					t.Error("owned lost proxy not recovered", result)
				}
			} else if len(result.Failures) != 1 || result.Failures[0].Outcome != want || len(engine.Resources) != 1 {
				t.Errorf("adoption %s = %#v", fault, result)
			}
		})
	}
}
func TestEgressStopAcceptsProxyAlreadyRemovedAfterWait(t *testing.T) {
	runner, registry, engine, id, r := seedEgressSession(t, 9)
	engine.AfterProxyWait = func() {
		delete(engine.Resources, r.Egress.Proxy.ID)
		for key, o := range engine.Networks {
			o.Endpoints, o.ContainerID = 0, ""
			engine.Networks[key] = o
		}
	}
	result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
	if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" {
		t.Errorf("confirmed absence refused: %#v", result)
	}
}
func TestEgressWatchRequiresConnectionAndSupportsOfflineCaller(t *testing.T) {
	runner, registry, engine, _, r := seedEgressSession(t, 8)
	b := registry.Store.Codec.(*f.Codec).Value
	c, failure := engine.OpenRun(t.Context(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	a, failure := c.Attach(t.Context(), r.ContainerID, port.Streams{})
	if failure != "" {
		t.Fatal(failure)
	}
	defer a.Close()
	engine.OpenNil = true
	if _, _, failure := runner.waitEgress(t.Context(), c, a, b, r); failure != port.Internal {
		t.Error("nil watch connection accepted", failure)
	}
	engine.OpenNil = false
	b.Egress = port.EgressGrant{}
	engine.Exit = 9
	if exit, exited, failure := runner.waitEgress(context.Background(), c, a, b, r); failure != "" || !exited || exit != 9 {
		t.Error("offline wait changed", exit, exited, failure)
	}
}
