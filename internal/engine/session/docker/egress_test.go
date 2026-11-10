package docker

import (
	"bytes"
	"errors"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSDKEgressContract(t *testing.T) {
	st.EgressConnectionContract(t, func() (sp.RunEngine, sp.Binding) { e, b, _, _ := egressSDKFixture(t); return e, b })
}
func TestCreateNetworkRequest(t *testing.T) {
	e, b, a, _ := egressSDKFixture(t)
	c, f := e.OpenRun(t.Context(), b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	rs := st.EgressRecords(b)
	for n, kind := range []sp.NetworkKind{sp.InternalNetwork, sp.EgressNetwork} {
		r := rs[1+n*2]
		id, f := c.CreateNetwork(t.Context(), r, kind)
		if f != "" || id != strings.Repeat(string(rune('a'+n)), 64) {
			t.Fatalf("create network = %q %s", id, f)
		}
		a.mu.Lock()
		body := a.networkBodies[n]
		a.mu.Unlock()
		name := r.Egress.Internal.Name
		if n == 1 {
			name = r.Egress.Network.Name
		}
		if body["Name"] != name || body["Driver"] != "bridge" || body["Internal"] != (n == 0) || body["EnableIPv6"] != false {
			t.Error("network body differs", body)
		}
		labels := body["Labels"].(map[string]any)
		if len(labels) != 5 || labels["io.itos-container.kind"] != string(kind) || labels["io.itos-container.intent"] != r.NameToken() || labels["io.itos-container.session"] != r.Anchor.Session || labels["io.itos-container.project"] != r.Project || labels["io.itos-container.fingerprint"] != r.Fingerprint {
			t.Error("labels differ", labels)
		}
		var wantOptions any
		if n == 0 {
			wantOptions = map[string]any{"com.docker.network.bridge.gateway_mode_ipv4": "isolated"}
		}
		if !reflect.DeepEqual(body["Options"], wantOptions) {
			t.Errorf("%s options = %#v, want %#v", kind, body["Options"], wantOptions)
		}
	}
	for _, r := range []sp.Record{rs[0], rs[2], rs[5]} {
		before := len(a.requests)
		if _, f := c.CreateNetwork(t.Context(), r, sp.InternalNetwork); f != sp.Refused || len(a.requests) != before {
			t.Error("bad intent sent request", f)
		}
	}
}
func TestInspectNetworkObservation(t *testing.T) {
	e, b, a, pin := egressSDKFixture(t)
	c, f := e.OpenRun(t.Context(), b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	r := st.EgressRecords(b)[2]
	a.networks[r.Egress.Internal.ID] = map[string]any{"Id": r.Egress.Internal.ID, "Name": r.Egress.Internal.Name, "Driver": "bridge", "Scope": "local", "Internal": true, "Options": map[string]string{"com.docker.network.bridge.gateway_mode_ipv4": "isolated"}, "Labels": map[string]string{"io.itos-container.intent": r.NameToken(), "io.itos-container.project": r.Project, "io.itos-container.fingerprint": r.Fingerprint, "io.itos-container.session": r.Anchor.Session, "io.itos-container.kind": "internal-network"}, "Containers": map[string]any{}}
	if o, s := c.InspectNetwork(t.Context(), r.Egress.Internal.ID); s != sp.Observed || o != st.EgressNetwork(r, sp.InternalNetwork) {
		t.Errorf("observation = %#v %v", o, s)
	}
	for _, field := range []string{"Id", "Name"} {
		a.networkDefect = func(v map[string]any) { delete(v, field) }
		if _, s := c.InspectNetwork(t.Context(), r.Egress.Internal.ID); s != sp.Unavailable {
			t.Error("missing identity accepted", field, s)
		}
	}
	a.networkDefect = nil
	if o, s := c.InspectNetwork(t.Context(), "missing"); s != sp.NotFound || o != (sp.NetworkObservation{}) {
		t.Error("404 not mapped", o, s)
	}
	pin.Current = false
	if _, s := c.InspectNetwork(t.Context(), r.Egress.Internal.ID); s != sp.Changed {
		t.Error("changed socket accepted", s)
	}
}
func TestCreateProxyRequest(t *testing.T) {
	e, b, a, _ := egressSDKFixture(t)
	c, f := e.OpenRun(t.Context(), b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	r := st.EgressRecords(b)[5]
	id, f := c.CreateProxy(t.Context(), r)
	if f != "" || !sp.HexID(id) {
		t.Fatalf("create proxy = %q %s", id, f)
	}
	a.mu.Lock()
	body := a.containerBodies[0]
	requests := slices.Clone(a.requests)
	a.mu.Unlock()
	if !slices.Equal(body.Cmd, []string{"/itos-container", "__egress-proxy", "--", "one.example", "two.example"}) || body.User != b.UID+":"+b.GID || body.Image != b.Egress.ImageID || body.WorkingDir != "/" || len(body.Entrypoint) != 0 || !slices.Equal(body.Env, []string{"IMAGE=retained", "HOME=/root"}) {
		t.Error("proxy config differs", body.Config)
	}
	h := body.HostConfig
	if h == nil || h.NetworkMode != container.NetworkMode(r.Egress.Network.Name) || !h.ReadonlyRootfs || !slices.Equal(h.CapDrop, []string{"ALL"}) || !slices.Equal(h.SecurityOpt, []string{"no-new-privileges"}) || h.PidsLimit == nil || *h.PidsLimit != 4096 || h.IpcMode != "private" || h.CgroupnsMode != "private" || h.Init == nil || *h.Init || h.RestartPolicy.Name != "no" || !reflect.DeepEqual(h.Sysctls, map[string]string{"net.ipv4.ip_forward": "0"}) || h.LogConfig.Type != "json-file" || !reflect.DeepEqual(h.LogConfig.Config, map[string]string{"max-size": "64m", "max-file": "1", "mode": "blocking"}) {
		t.Error("proxy host config differs", h)
	}
	if len(h.Mounts) != 1 || h.Mounts[0].Source != e.Executable || h.Mounts[0].Target != "/itos-container" || !h.Mounts[0].ReadOnly || h.Mounts[0].BindOptions != nil {
		t.Error("proxy mount differs", h.Mounts)
	}
	want := map[string]*network.EndpointSettings{r.Egress.Internal.Name: {NetworkID: r.Egress.Internal.ID}, r.Egress.Network.Name: {NetworkID: r.Egress.Network.ID}}
	if body.NetworkingConfig == nil || !reflect.DeepEqual(body.NetworkingConfig.EndpointsConfig, want) {
		t.Error("network IDs not pinned", body.NetworkingConfig)
	}
	if !slices.Equal(requests, []string{"GET /images/" + b.Egress.ImageID + "/json", "POST /containers/create"}) {
		t.Error("image not inspected before create", requests)
	}
}

type failedLogWriter struct{}

func (failedLogWriter) Write([]byte) (int, error) { return 0, errors.New("failed sink") }
func TestProxyReadyReadsStdoutOnly(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		want       sp.Outcome
	}{
		{"ready", strings.SplitAfter(st.ProxyStdout, "\n")[0], ""},
		{"stderr looking stdout", "failure\n", sp.ProxyFailed},
		{"early end", "{", sp.ProxyFailed},
		{"long", strings.Repeat(" ", 2049) + "\n", sp.ProxyFailed},
		{"extra key", `{"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready","extra":0}` + "\n", sp.ProxyFailed},
		{"wrong seq", strings.Replace(strings.SplitAfter(st.ProxyStdout, "\n")[0], `"seq":1`, `"seq":2`, 1), sp.ProxyFailed},
		{"wrong event", strings.Replace(strings.SplitAfter(st.ProxyStdout, "\n")[0], "ready", "refused", 1), sp.ProxyFailed},
		{"wrong time", strings.Replace(strings.SplitAfter(st.ProxyStdout, "\n")[0], "Z", "+00:00", 1), sp.ProxyFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, b, a, _ := egressSDKFixture(t)
			c, f := e.OpenRun(t.Context(), b)
			if f != "" {
				t.Fatal(f)
			}
			defer c.Close()
			id := strings.Repeat("c", 64)
			a.containers[id] = &container.InspectResponse{}
			a.logs = append(logFrame(2, "not JSON\n"), logFrame(1, tc.line)...)
			if f := c.ProxyReady(t.Context(), id); f != tc.want {
				t.Errorf("ready = %s, want %s", f, tc.want)
			}
			if len(a.logQueries) != 1 {
				t.Fatalf("want one log call, got %d", len(a.logQueries))
			}
			q, _ := url.ParseQuery(a.logQueries[0])
			if q.Get("stdout") != "1" || q.Get("stderr") == "1" || q.Get("timestamps") == "1" || q.Get("details") == "1" || q.Get("follow") != "1" {
				t.Error("unsafe ready query", q)
			}
			a.logs = logFrame(3, "daemon read failure")
			if f := c.ProxyReady(t.Context(), id); f != sp.ExecutionFailed {
				t.Error("system stream error lost", f)
			}
			a.logFailure = true
			if f := c.ProxyReady(t.Context(), id); f != sp.ExecutionFailed {
				t.Error("request error lost", f)
			}
		})
	}
}
func TestCopyProxyLogCopiesStdoutOnly(t *testing.T) {
	e, b, a, _ := egressSDKFixture(t)
	c, f := e.OpenRun(t.Context(), b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	id := strings.Repeat("c", 64)
	a.containers[id] = &container.InspectResponse{}
	a.logs = append(append(logFrame(1, "first\n"), logFrame(2, "not JSON\n")...), logFrame(1, "last\n")...)
	var out bytes.Buffer
	if f = c.CopyProxyLog(t.Context(), id, &out); f != "" || out.String() != "first\nlast\n" {
		t.Errorf("stdout copy = %q %s", out.String(), f)
	}
	if len(a.logQueries) != 1 {
		t.Fatalf("want one log call")
	}
	q, _ := url.ParseQuery(a.logQueries[0])
	if q.Get("stdout") != "1" || q.Get("stderr") == "1" || q.Get("follow") == "1" || q.Get("timestamps") == "1" || q.Get("details") == "1" {
		t.Error("unsafe copy query", q)
	}
	if f = c.CopyProxyLog(t.Context(), id, failedLogWriter{}); f != sp.ExecutionFailed {
		t.Error("write failure lost", f)
	}
	a.logs = logFrame(3, "daemon read failure")
	if f = c.CopyProxyLog(t.Context(), id, &out); f != sp.ExecutionFailed {
		t.Error("system error lost", f)
	}
}
