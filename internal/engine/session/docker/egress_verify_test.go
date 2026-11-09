package docker

import (
	"bytes"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func TestVerifyProxyRefusesEachDefect(t *testing.T) {
	defects := map[string]func(*container.InspectResponse){
		"argv":                  func(r *container.InspectResponse) { r.Config.Cmd = []string{"sh"} },
		"environment":           func(r *container.InspectResponse) { r.Config.Env = append(r.Config.Env, "UNAPPROVED=1") },
		"stdin":                 func(r *container.InspectResponse) { r.Config.OpenStdin = true },
		"tty":                   func(r *container.InspectResponse) { r.Config.Tty = true },
		"root":                  func(r *container.InspectResponse) { r.Config.User = "0:0" },
		"directory":             func(r *container.InspectResponse) { r.Config.WorkingDir = "/tmp" },
		"rootfs":                func(r *container.InspectResponse) { r.HostConfig.ReadonlyRootfs = false },
		"privileged":            func(r *container.InspectResponse) { r.HostConfig.Privileged = true },
		"capadd":                func(r *container.InspectResponse) { r.HostConfig.CapAdd = []string{"NET_ADMIN"} },
		"capdrop":               func(r *container.InspectResponse) { r.HostConfig.CapDrop = nil },
		"sysctl":                func(r *container.InspectResponse) { r.HostConfig.Sysctls["net.ipv4.ip_forward"] = "1" },
		"extra sysctl":          func(r *container.InspectResponse) { r.HostConfig.Sysctls["other"] = "0" },
		"writable executable":   func(r *container.InspectResponse) { r.HostConfig.Mounts[0].ReadOnly = false },
		"mount source":          func(r *container.InspectResponse) { r.HostConfig.Mounts[0].Source = "/other" },
		"materialized writable": func(r *container.InspectResponse) { r.Mounts[0].RW = true },
		"network ID": func(r *container.InspectResponse) {
			for _, ep := range r.NetworkSettings.Networks {
				ep.NetworkID = ""
				break
			}
		},
		"nil endpoint": func(r *container.InspectResponse) {
			for name := range r.NetworkSettings.Networks {
				r.NetworkSettings.Networks[name] = nil
				break
			}
		},
		"extra network": func(r *container.InspectResponse) {
			r.NetworkSettings.Networks["foreign"] = &network.EndpointSettings{}
		},
		"log nonblocking": func(r *container.InspectResponse) { r.HostConfig.LogConfig.Config["mode"] = "non-blocking" },
		"log size":        func(r *container.InspectResponse) { r.HostConfig.LogConfig.Config["max-size"] = "1m" },
		"log rotate":      func(r *container.InspectResponse) { r.HostConfig.LogConfig.Config["max-file"] = "2" },
		"started":         func(r *container.InspectResponse) { r.State.StartedAt = "2026-11-09T12:00:00Z" },
	}
	for name, defect := range defects {
		t.Run(name, func(t *testing.T) {
			e, b, a, _ := egressSDKFixture(t)
			c, r := createEgressFixture(t, e, b)
			if f := c.VerifyProxy(t.Context(), r.Egress.Proxy.ID); f != "" {
				t.Fatalf("healthy proxy verification = %s", f)
			}
			a.mu.Lock()
			a.defect = defect
			a.mu.Unlock()
			if f := c.VerifyProxy(t.Context(), r.Egress.Proxy.ID); f != sp.IsolationUnverified {
				t.Error("proxy defect accepted", name, f)
			}
		})
	}
}
func TestCreateRunOnInternalNetwork(t *testing.T) {
	e, b, a, _ := egressSDKFixture(t)
	c, r := createEgressFixture(t, e, b)
	if f := c.Start(t.Context(), r.Egress.Proxy.ID); f != "" {
		t.Fatal(f)
	}
	r.Stage = "intent"
	if _, f := c.CreateRun(t.Context(), r, []string{"probe"}); f != "" {
		t.Fatalf("worker create = %s", f)
	}
	body := a.containerBodies[1]
	if body.HostConfig.NetworkMode != container.NetworkMode(r.Egress.Internal.Name) || body.NetworkingConfig == nil || len(body.NetworkingConfig.EndpointsConfig) != 1 || body.NetworkingConfig.EndpointsConfig[r.Egress.Internal.Name].NetworkID != r.Egress.Internal.ID {
		t.Error("worker network differs", body)
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		if !slices.Contains(body.Env, name+"=http://172.20.0.2:3128") {
			t.Error("missing proxy environment", name, body.Env)
		}
	}
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		if !slices.Contains(body.Env, name+"=localhost,127.0.0.1,::1") {
			t.Error("missing loopback bypass", name)
		}
	}
	before := len(a.requests)
	r.Schema = 2
	if _, f := c.CreateRun(t.Context(), r, []string{"probe"}); f != sp.Refused || len(a.requests) != before {
		t.Error("grant accepted offline worker", f)
	}
}
func TestVerifyRunEgressNetworks(t *testing.T) {
	for _, fault := range []string{"extra", "nil", "missing ID", "wrong ID", "DNS", "DNS search", "DNS options", "port"} {
		t.Run(fault, func(t *testing.T) {
			e, b, a, _ := egressSDKFixture(t)
			c, r := createEgressFixture(t, e, b)
			if f := c.Start(t.Context(), r.Egress.Proxy.ID); f != "" {
				t.Fatal(f)
			}
			r.Stage = "intent"
			id, f := c.CreateRun(t.Context(), r, []string{"probe"})
			if f != "" {
				t.Fatal(f)
			}
			if f := c.VerifyRun(t.Context(), id, []string{"probe"}); f != "" {
				t.Fatalf("healthy worker = %s", f)
			}
			a.defect = func(v *container.InspectResponse) {
				switch fault {
				case "extra":
					v.NetworkSettings.Networks["foreign"] = &network.EndpointSettings{}
				case "nil":
					v.NetworkSettings.Networks[r.Egress.Internal.Name] = nil
				case "missing ID":
					v.NetworkSettings.Networks[r.Egress.Internal.Name].NetworkID = ""
				case "wrong ID":
					v.NetworkSettings.Networks[r.Egress.Internal.Name].NetworkID = strings.Repeat("e", 64)
				case "DNS":
					v.HostConfig.DNS = []netip.Addr{netip.MustParseAddr("1.1.1.1")}
				case "DNS search":
					v.HostConfig.DNSSearch = []string{"foreign"}
				case "DNS options":
					v.HostConfig.DNSOptions = []string{"ndots:0"}
				case "port":
					v.HostConfig.PublishAllPorts = true
				}
			}
			if f := c.VerifyRun(t.Context(), id, []string{"probe"}); f != sp.IsolationUnverified {
				t.Error("worker boundary defect accepted", fault, f)
			}
		})
	}
}
func TestEgressPreStartNetworkIDs(t *testing.T) {
	e, b, a, _ := egressSDKFixture(t)
	c, r := createEgressFixture(t, e, b)
	if f := c.VerifyProxy(t.Context(), r.Egress.Proxy.ID); f != "" {
		t.Fatalf("proxy verify before start = %s", f)
	}
	for _, ep := range a.containers[r.Egress.Proxy.ID].NetworkSettings.Networks {
		if ep.EndpointID != "" || ep.IPAddress.IsValid() {
			t.Fatal("fixture allocated endpoint before start")
		}
	}
	for _, bad := range []string{"", strings.Repeat("f", 64)} {
		ep := a.containers[r.Egress.Proxy.ID].NetworkSettings.Networks[r.Egress.Internal.Name]
		old := ep.NetworkID
		ep.NetworkID = bad
		if f := c.VerifyProxy(t.Context(), r.Egress.Proxy.ID); f != sp.IsolationUnverified {
			t.Error("proxy omitted ID accepted", f)
		}
		ep.NetworkID = old
	}
	if f := c.Start(t.Context(), r.Egress.Proxy.ID); f != "" {
		t.Fatal(f)
	}
	if f := c.ProxyReady(t.Context(), r.Egress.Proxy.ID); f != "" {
		t.Fatal(f)
	}
	r.Stage = "intent"
	id, f := c.CreateRun(t.Context(), r, []string{"probe"})
	if f != "" {
		t.Fatal(f)
	}
	if f := c.VerifyRun(t.Context(), id, []string{"probe"}); f != "" {
		t.Error("worker verify before start", f)
	}
	for _, bad := range []string{"", strings.Repeat("f", 64)} {
		ep := a.containers[id].NetworkSettings.Networks[r.Egress.Internal.Name]
		old := ep.NetworkID
		ep.NetworkID = bad
		if f := c.VerifyRun(t.Context(), id, []string{"probe"}); f != sp.IsolationUnverified {
			t.Error("worker omitted ID accepted", f)
		}
		ep.NetworkID = old
	}
}
func TestEgressRequestsRecheckSocket(t *testing.T) {
	e, b, a, pin := egressSDKFixture(t)
	c, f := e.OpenRun(t.Context(), b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	r := st.EgressRecords(b)[1]
	if _, f := c.CreateNetwork(t.Context(), r, sp.InternalNetwork); f != "" {
		t.Fatalf("healthy network control: %s", f)
	}
	pin.Current = false
	before := len(a.requests)
	if _, f := c.CreateNetwork(t.Context(), r, sp.InternalNetwork); f != sp.Refused {
		t.Error(f)
	}
	if _, s := c.InspectNetwork(t.Context(), r.Egress.Internal.Name); s != sp.Changed {
		t.Error(s)
	}
	c.RemoveNetwork(t.Context(), strings.Repeat("a", 64))
	r = st.EgressRecords(b)[5]
	if _, f := c.CreateProxy(t.Context(), r); f != sp.Refused {
		t.Error(f)
	}
	if f := c.VerifyProxy(t.Context(), strings.Repeat("c", 64)); f != sp.Refused {
		t.Error(f)
	}
	if f := c.ProxyReady(t.Context(), strings.Repeat("c", 64)); f != sp.Refused {
		t.Error(f)
	}
	if f := c.CopyProxyLog(t.Context(), strings.Repeat("c", 64), &bytes.Buffer{}); f != sp.Refused {
		t.Error(f)
	}
	if len(a.requests) != before {
		t.Error("request sent after socket changed")
	}
}
