package docker

import (
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/container"
	"net/http/httptrace"
	"net/netip"
	"testing"
)

func TestEgressWorkerRequiresLivePinnedProxy(t *testing.T) {
	for _, fault := range []string{"invalid record", "changed before", "changed after", "unavailable", "config missing", "state missing", "network missing", "not running", "foreign", "network ID", "address", "IPv6"} {
		t.Run(fault, func(t *testing.T) {
			e, b, a, pin := egressSDKFixture(t)
			c, r := createEgressFixture(t, e, b)
			if failure := c.Start(t.Context(), r.Egress.Proxy.ID); failure != "" {
				t.Fatal(failure)
			}
			r.Stage = "intent"
			want := sp.IsolationUnverified
			ctx := t.Context()
			switch fault {
			case "invalid record":
				r.Egress.Proxy.Stage = "intent"
				r.Egress.Proxy.ID = ""
				want = sp.Refused
			case "changed before":
				pin.Current = false
				want = sp.Refused
			case "changed after":
				ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotFirstResponseByte: func() { pin.Current = false }})
				want = sp.Refused
			case "unavailable":
				a.replyStatus = 500
			default:
				a.defect = func(v *container.InspectResponse) {
					switch fault {
					case "config missing":
						v.Config = nil
					case "state missing":
						v.State = nil
					case "network missing":
						v.NetworkSettings = nil
					case "not running":
						v.State.Running = false
					case "foreign":
						v.Config.Labels["io.itos-container.kind"] = "other"
					case "network ID":
						v.NetworkSettings.Networks[r.Egress.Internal.Name].NetworkID = "other"
					case "address":
						v.NetworkSettings.Networks[r.Egress.Internal.Name].IPAddress = netip.Addr{}
					case "IPv6":
						v.NetworkSettings.Networks[r.Egress.Internal.Name].IPAddress = netip.MustParseAddr("::1")
					}
				}
			}
			// Exercise the pre-create inspection independently as well as the public
			// create refusal, so the socket pin is required at this boundary itself.
			if _, _, failure := c.(*runConnection).workerNetwork(ctx, r); failure != want {
				t.Error("proxy check", fault, failure, want)
			}
			if _, failure := c.CreateRun(ctx, r, []string{"probe"}); failure == "" {
				t.Error("unsafe proxy permitted worker create", fault)
			}
			if len(a.containerBodies) != 1 {
				t.Error("unsafe worker create sent", fault)
			}
		})
	}
}
func TestEgressNetworkInspectionCarriesEndpointAndFlags(t *testing.T) {
	e, b, a, pin := egressSDKFixture(t)
	c, r := createEgressFixture(t, e, b)
	if failure := c.Start(t.Context(), r.Egress.Proxy.ID); failure != "" {
		t.Fatal(failure)
	}
	o, status := c.InspectNetwork(t.Context(), r.Egress.Internal.ID)
	if status != sp.Observed || o.Endpoints != 1 || o.ContainerID != r.Egress.Proxy.ID {
		t.Error("endpoint identity lost", o, status)
	}
	a.networkDefect = func(v map[string]any) {
		v["ConfigOnly"], v["Ingress"], v["Attachable"], v["EnableIPv6"] = true, true, true, true
		v["ConfigFrom"] = map[string]string{"Network": "other"}
		v["Labels"].(map[string]any)["extra"] = "extra"
	}
	o, status = c.InspectNetwork(t.Context(), r.Egress.Internal.ID)
	if status != sp.Observed || !o.ConfigOnly || !o.Ingress || !o.Attachable || !o.IPv6 || !o.FromConfig || o.ExactLabels {
		t.Error("network flags lost", o, status)
	}
	ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{GotFirstResponseByte: func() { pin.Current = false }})
	if _, status := c.InspectNetwork(ctx, r.Egress.Internal.ID); status != sp.Changed {
		t.Error("socket change after inspection lost", status)
	}
	pin.Current = true
	a.networkDefect = nil
	record := st.EgressRecords(b)[1]
	for _, kind := range []sp.NetworkKind{sp.EgressNetwork, "unknown"} {
		before := len(a.requests)
		if _, failure := c.CreateNetwork(t.Context(), record, kind); failure != sp.Refused || len(a.requests) != before {
			t.Error("out-of-order network created", kind, failure)
		}
	}
}
