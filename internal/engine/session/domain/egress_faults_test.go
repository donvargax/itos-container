package domain

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"slices"
	"testing"
)

func TestEgressStartupKeepsDurableFailures(t *testing.T) {
	for _, tc := range []struct {
		fault string
		want  port.Outcome
	}{
		{"internal-network short ID", port.Uncertain}, {"egress-network short ID", port.Uncertain}, {"proxy short ID", port.Uncertain},
		{"network unavailable", port.IsolationInspection}, {"network changed", port.Refused}, {"network foreign", port.Refused},
		{"proxy inspection unavailable", port.IsolationInspection}, {"proxy inspection foreign", port.Refused}, {"proxy inspection changed", port.Refused},
		{"intent write", port.IntentWrite}, {"egress intent write", port.IntentWrite}, {"proxy intent write", port.IntentWrite}, {"internal ownership write", port.OwnershipWrite}, {"egress ownership write", port.OwnershipWrite}, {"proxy ownership write", port.OwnershipWrite},
		{"network daemon changed", port.Refused}, {"proxy daemon changed", port.Refused},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			runner, registry, engine, id := egressSessionFixture(t)
			store := registry.Store
			engine.EgressFault = tc.fault
			engine.After = func(event string) {
				switch {
				case event == "proxy create" && tc.fault == "proxy inspection changed":
					engine.BeforeInspect = func() { engine.Replaced = true }
				case event == "proxy create" && tc.fault == "proxy inspection unavailable":
					engine.Inspection = "unavailable"
				case event == "proxy create" && tc.fault == "proxy inspection foreign":
					engine.Inspection = "different name"
				case event == "internal-network create" && tc.fault == "internal ownership write", event == "egress-network create" && tc.fault == "egress ownership write", event == "proxy create" && tc.fault == "proxy ownership write":
					store.Fault = "reserved-record write"
				}
			}
			store.AfterSave = func() {
				for _, data := range store.Records {
					r, _ := store.Codec.Decode(data)
					if tc.fault == "intent write" && r.Schema == 2 && r.Anchor.Stage == "owned" {
						store.Fault = "reserved-record write"
					}
					if tc.fault == "proxy intent write" && r.Egress.Network.Stage == "owned" {
						store.Fault = "reserved-record write"
					}
					if tc.fault == "egress intent write" && r.Egress.Internal.Stage == "owned" {
						store.Fault = "reserved-record write"
					}
					if tc.fault == "network daemon changed" && r.Schema == 3 && r.Egress.Internal.Stage == "intent" {
						engine.DaemonID = "foreign"
					}
					if tc.fault == "proxy daemon changed" && r.Schema == 3 && r.Egress.Proxy.Stage == "intent" {
						engine.DaemonID = "foreign"
					}
				}
			}
			if failure := launchEgress(t, runner, id); failure != tc.want || slices.Contains(engine.Events, "start") {
				t.Errorf("startup %s = %s, want %s; %v", tc.fault, failure, tc.want, engine.Events)
			}
			if tc.fault == "proxy intent write" && slices.Contains(engine.Events, "proxy create") {
				t.Error("proxy created without durable intent")
			}
			if tc.fault == "egress intent write" && slices.Contains(engine.Events, "egress-network create") {
				t.Error("egress network created without durable intent")
			}
			if tc.fault == "proxy inspection changed" && !slices.ContainsFunc(engine.Engine.Events, func(event f.EngineEvent) bool {
				return event.Method == "INSPECT" && event.Target == engine.ProxyID && event.Status == port.Changed
			}) {
				t.Error("socket replacement never reached proxy inspection")
			}
		})
	}
}
func TestEgressRechecksNetworkAndProxyAfterWorkerCreate(t *testing.T) {
	for _, tc := range []struct {
		fault string
		want  port.Outcome
	}{
		{"network unavailable", port.IsolationInspection}, {"network changed", port.Refused}, {"network foreign", port.Refused}, {"gateway", port.IsolationUnverified}, {"foreign endpoint", port.IsolationUnverified}, {"proxy stopped", port.ProxyFailed},
		{"internal-network foreign single endpoint", port.IsolationUnverified}, {"egress-network foreign single endpoint", port.IsolationUnverified},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			runner, _, engine, id := egressSessionFixture(t)
			engine.After = func(event string) {
				if event != "create" {
					return
				}
				engine.EgressFault = tc.fault
				if tc.fault == "gateway" {
					for key, o := range engine.Networks {
						if o.Kind == string(port.InternalNetwork) {
							o.Gateway = true
							engine.Networks[key] = o
						}
					}
				}
				if tc.fault == "proxy stopped" {
					o := engine.Resources[engine.ProxyID]
					o.Running, o.Status, o.PID = false, "exited", 0
					engine.Resources[engine.ProxyID] = o
				}
			}
			if failure := launchEgress(t, runner, id); failure != tc.want || slices.Contains(engine.Events, "start") || !slices.Contains(engine.Events, "create") {
				t.Errorf("pre-start check %s = %s, want %s; %v", tc.fault, failure, tc.want, engine.Events)
			}
		})
	}
}
func TestEgressRecoveryChecksNetworkOwnershipAndPresence(t *testing.T) {
	for _, fault := range []string{"foreign", "unavailable", "changed", "endpoints", "absent present", "intent foreign"} {
		t.Run(fault, func(t *testing.T) {
			runner, registry, engine, id, r := seedEgressSession(t, 9)
			if fault == "absent present" {
				r.Egress.Proxy.Stage = "absent"
				registry.Store.Records[registry.Records[id].Intent] = registry.Store.Codec.Encode(r)
			}
			if fault == "intent foreign" {
				r.Egress.Proxy.Stage = "intent"
				r.Egress.Proxy.ID = ""
				registry.Store.Records[registry.Records[id].Intent] = registry.Store.Codec.Encode(r)
				o := engine.Resources[engine.ProxyID]
				o.Kind = "foreign"
				engine.Resources[engine.ProxyID] = o
			}
			engine.After = func(event string) {
				if event != "proxy remove" {
					return
				}
				switch fault {
				case "foreign":
					engine.EgressFault = "network foreign"
				case "unavailable":
					engine.EgressFault = "network unavailable"
				case "changed":
					engine.EgressFault = "network changed"
				case "endpoints":
					for key, o := range engine.Networks {
						o.Endpoints = 2
						o.ContainerID = ""
						engine.Networks[key] = o
					}
				}
			}
			result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
			want := port.Refused
			if fault == "unavailable" || fault == "endpoints" {
				want = port.Incomplete
			}
			if len(result.Failures) != 1 || result.Failures[0].Outcome != want || len(engine.Networks) != 2 || len(engine.Anchors) != 1 {
				t.Errorf("recovery %s = %#v", fault, result)
			}
			before := registry.Writes
			status := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Status(t.Context())
			if fault != "endpoints" && (len(status.Failures) != 1 || status.Failures[0].Outcome != want) {
				t.Errorf("status lost ownership failure: %#v", status)
			}
			if registry.Writes != before {
				t.Error("status rewrote ownership")
			}
		})
	}
}
func TestEgressRecoveryRequiresAbsentWorker(t *testing.T) {
	runner, registry, engine, id, _ := seedEgressSession(t, 8)
	s, failure := registry.Open(t.Context(), id, false)
	if failure != "" {
		t.Fatal(failure)
	}
	defer s.Close()
	b := registry.Store.Codec.(*f.Codec).Value
	c, failure := engine.OpenRun(t.Context(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	if failure := releaseEgress(t.Context(), s.Intent(), c); failure != port.Refused {
		t.Error("released live worker's network", failure)
	}
	if failure := releaseAnchor(t.Context(), s.Intent(), c); failure != port.Refused {
		t.Error("released live egress anchor", failure)
	}
	_ = runner
}
