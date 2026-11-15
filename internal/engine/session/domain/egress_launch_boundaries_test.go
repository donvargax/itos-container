package domain

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func openEgressTestSession(t *testing.T, registry *f.Registry, engine *f.RunEngine, id string) (port.Intent, port.RunConnection) {
	t.Helper()
	s, failure := registry.Open(t.Context(), id, false)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(s.Close)
	c, failure := engine.OpenRun(t.Context(), s.Intent().Binding())
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(c.Close)
	return s.Intent(), c
}

func TestEgressStartupOperationPreservesDiagnostics(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		for _, failure := range []port.Outcome{"", port.Refused} {
			t.Run(fmt.Sprintf("tracked=%v/failure=%s", tracked, failure), func(t *testing.T) {
				ctx := t.Context()
				before := startupState{operation: "previous", failure: port.IsolationUnverified}
				state := before
				if tracked {
					ctx = context.WithValue(ctx, startupKey{}, &state)
				}
				calls := 0
				got := egressStartupOperation(ctx, "network creation", func(context.Context) port.Outcome {
					calls++
					return failure
				})
				want := before
				if tracked && failure != "" {
					want.operation, want.failure = "network creation", failure
				}
				if got != failure || calls != 1 || state.operation != want.operation || state.failure != want.failure {
					t.Fatalf("operation = %s, calls %d, diagnostic %s/%s; want %s/%s", got, calls, state.operation, state.failure, want.operation, want.failure)
				}
			})
		}
	}
}

func TestEgressPreStartEndpointCounts(t *testing.T) {
	for _, kind := range []port.NetworkKind{port.InternalNetwork, port.EgressNetwork} {
		for _, count := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/%d", kind, count), func(t *testing.T) {
				_, registry, engine, id, r := seedEgressSession(t, 6)
				i, c := openEgressTestSession(t, registry, engine, id)
				networkID := r.Egress.Internal.ID
				if kind == port.EgressNetwork {
					networkID = r.Egress.Network.ID
				}
				o := engine.Networks[networkID]
				// Keep the identity correct so only the endpoint count decides.
				o.Endpoints, o.ContainerID = count, r.Egress.Proxy.ID
				engine.Networks[networkID] = o
				want := port.IsolationUnverified
				if count == 1 {
					want = ""
				}
				if got := verifyEgressStart(t.Context(), i, c); got != want {
					t.Fatalf("pre-start = %s, want %s", got, want)
				}
			})
		}
	}
}

func TestEgressLiveProxyRequiresEveryRunningFact(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*port.Observation)
	}{
		{"healthy", func(*port.Observation) {}},
		{"foreign", func(o *port.Observation) { o.Kind = "foreign" }},
		{"missing state", func(o *port.Observation) { o.StatePresent = false }},
		{"not running", func(o *port.Observation) { o.Running = false }},
		{"wrong status", func(o *port.Observation) { o.Status = "paused" }},
		{"missing", nil},
		{"unavailable", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, registry, engine, id, r := seedEgressSession(t, 6)
			i, c := openEgressTestSession(t, registry, engine, id)
			o := engine.Resources[r.Egress.Proxy.ID]
			switch tc.name {
			case "missing":
				delete(engine.Resources, o.ID)
			case "unavailable":
				engine.Inspection = "unavailable"
			default:
				tc.edit(&o)
				engine.Resources[r.Egress.Proxy.ID] = o
			}
			want := port.ProxyFailed
			if tc.name == "healthy" {
				want = ""
			}
			state := &startupState{}
			ctx := context.WithValue(t.Context(), startupKey{}, state)
			if got := verifyLiveProxy(ctx, r, i.Binding(), c); got != want {
				t.Fatalf("live proxy = %s, want %s", got, want)
			}
			if want != "" && (state.operation != "proxy inspection" || state.failure != want) {
				t.Fatal("lost live-proxy diagnostic", state.operation, state.failure)
			}
		})
	}
}

func TestEgressCreateInspectionMustMatchReturnedID(t *testing.T) {
	for _, resource := range []string{"internal-network", "egress-network", "proxy"} {
		t.Run(resource, func(t *testing.T) {
			runner, _, engine, id := egressSessionFixture(t)
			created := false
			engine.After = func(event string) {
				if event == resource+" create" {
					created = true
				}
				if created && event == "network inspect" && resource != "proxy" {
					for key, o := range engine.Networks {
						if o.Kind == resource {
							o.ID = strings.Repeat("f", 64)
							engine.Networks[key] = o
						}
					}
				}
			}
			engine.BeforeInspect = func() {
				if created && resource == "proxy" {
					o := engine.Resources[engine.ProxyID]
					o.ID = strings.Repeat("f", 64)
					engine.Resources[engine.ProxyID] = o
				}
			}
			if got := launchEgress(t, runner, id); got != port.Refused || !created || slices.Contains(engine.Events, "create") {
				t.Fatalf("different inspected ID = %s, events %v", got, engine.Events)
			}
		})
	}
}
