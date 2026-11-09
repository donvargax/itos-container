package docker

import (
	"bytes"
	"context"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/container"
	"net/http/httptrace"
	"strings"
	"testing"
)

func TestEgressCreateRefusalsAndUncertainReplies(t *testing.T) {
	for _, operation := range []string{"network", "proxy"} {
		for _, fault := range []string{"invalid record", "wrong stage", "request error", "malformed response", "short ID", "socket after request", "missing image config", "image volume", "image ID", "relative executable"} {
			t.Run(operation+"/"+fault, func(t *testing.T) {
				e, b, a, pin := egressSDKFixture(t)
				if fault == "relative executable" {
					e.Executable = "relative"
				}
				c, f := e.OpenRun(t.Context(), b)
				if f != "" {
					t.Fatal(f)
				}
				defer c.Close()
				r := st.EgressRecords(b)[1]
				if operation == "proxy" {
					r = st.EgressRecords(b)[5]
				}
				want := sp.Refused
				switch fault {
				case "invalid record":
					r.Schema = 2
				case "wrong stage":
					r.Stage = "owned"
				case "request error":
					a.replyStatus = 500
					want = sp.Uncertain
				case "malformed response":
					a.malformed = true
					want = sp.Uncertain
				case "short ID":
					a.shortID = true
					want = sp.Uncertain
				case "missing image config":
					a.imageDefect = func(v map[string]any) { delete(v, "Config") }
					want = sp.Uncertain
				case "image volume":
					a.imageDefect = func(v map[string]any) {
						v["Config"].(map[string]any)["Volumes"] = map[string]any{"/data": map[string]any{}}
					}
				case "image ID":
					a.imageDefect = func(v map[string]any) { v["Id"] = "other" }
				}
				ctx := t.Context()
				if fault == "socket after request" {
					ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotFirstResponseByte: func() { pin.Current = false }})
					if operation == "network" {
						want = sp.Uncertain
					}
				}
				if operation == "network" && (strings.HasPrefix(fault, "image") || fault == "missing image config" || fault == "relative executable") {
					want = ""
				}
				var id string
				if operation == "network" {
					id, f = c.CreateNetwork(ctx, r, sp.InternalNetwork)
				} else {
					id, f = c.CreateProxy(ctx, r)
				}
				if f != want || (want != "" && id != "") {
					t.Errorf("%s = %q %s, want %s", fault, id, f, want)
				}
				if fault == "invalid record" || fault == "wrong stage" || fault == "relative executable" && operation == "proxy" {
					if len(a.requests) != 0 {
						t.Error("refusal sent request", a.requests)
					}
				}
			})
		}
	}
}
func TestEgressInspectionFailuresAndChangedSocket(t *testing.T) {
	for _, fault := range []string{"unavailable", "malformed", "config missing", "host missing", "state missing", "network missing", "foreign ID", "foreign image", "foreign name", "foreign label", "socket"} {
		t.Run(fault, func(t *testing.T) {
			e, b, a, pin := egressSDKFixture(t)
			c, r := createEgressFixture(t, e, b)
			want := sp.IsolationInspection
			switch fault {
			case "unavailable":
				a.replyStatus = 500
			case "malformed":
				a.malformed = true
			case "config missing":
				a.defect = func(v *container.InspectResponse) { v.Config = nil }
			case "host missing":
				a.defect = func(v *container.InspectResponse) { v.HostConfig = nil }
			case "state missing":
				a.defect = func(v *container.InspectResponse) { v.State = nil }
			case "network missing":
				a.defect = func(v *container.InspectResponse) { v.NetworkSettings = nil }
			case "foreign ID":
				a.defect = func(v *container.InspectResponse) { v.ID = "other" }
				want = sp.Refused
			case "foreign image":
				a.defect = func(v *container.InspectResponse) { v.Image = "other" }
				want = sp.Refused
			case "foreign name":
				a.defect = func(v *container.InspectResponse) { v.Name = "/other" }
				want = sp.Refused
			case "foreign label":
				a.defect = func(v *container.InspectResponse) { v.Config.Labels["io.itos-container.kind"] = "other" }
				want = sp.Refused
			}
			ctx := t.Context()
			if fault == "socket" {
				ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotFirstResponseByte: func() { pin.Current = false }})
				want = sp.Refused
			}
			if f := c.VerifyProxy(ctx, r.Egress.Proxy.ID); f != want {
				t.Error("proxy inspection failure", f, want)
			}
		})
	}
}
func TestEgressLogFaultBoundaries(t *testing.T) {
	e, b, a, pin := egressSDKFixture(t)
	c, f := e.OpenRun(t.Context(), b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	id := strings.Repeat("c", 64)
	a.containers[id] = &container.InspectResponse{}
	ready := strings.TrimSuffix(strings.SplitAfter(st.ProxyStdout, "\n")[0], "\n")
	for _, size := range []int{2048, 2049} {
		a.logs = logFrame(1, strings.Repeat(" ", size-len(ready))+ready+"\n")
		want := sp.Outcome("")
		if size == 2049 {
			want = sp.ProxyFailed
		}
		if f := c.ProxyReady(t.Context(), id); f != want {
			t.Errorf("%d byte line = %s", size, f)
		}
	}
	for _, line := range []string{"[]", "{", "{\"schema\":", `{"schema":1,"schema":1,"seq":1,"time":"2026-11-09T12:00:00Z","event":"ready"}`, `{"schema":1,"seq":1,"time":"invalidZ","event":"ready"}`, ready + " ", strings.Replace(ready, `"schema":1`, `"schema":2`, 1)} {
		a.logs = logFrame(1, line+"\n")
		if f := c.ProxyReady(t.Context(), id); f != sp.ProxyFailed {
			t.Error("invalid first line accepted", line, f)
		}
	}
	a.logs = append(append(logFrame(1, ready[:5]), logFrame(2, "panic\n")...), logFrame(1, ready[5:]+"\n")...)
	if f := c.ProxyReady(t.Context(), id); f != "" {
		t.Error("split stdout line refused", f)
	}
	for _, operation := range []func(context.Context) sp.Outcome{func(ctx context.Context) sp.Outcome { return c.ProxyReady(ctx, id) }, func(ctx context.Context) sp.Outcome { return c.CopyProxyLog(ctx, id, &bytes.Buffer{}) }} {
		a.logFailure = true
		pin.Current = true
		changed := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{GotFirstResponseByte: func() { pin.Current = false }})
		if f := operation(changed); f != sp.Refused {
			t.Error("socket replacement hidden by log error", f)
		}
		pin.Current = true
		a.logFailure = false
		if f := operation(changed); f != sp.Refused {
			t.Error("socket replacement hidden by log success", f)
		}
	}
	pin.Current = true
	a.logFailure = true
	if f := c.CopyProxyLog(t.Context(), id, &bytes.Buffer{}); f != sp.ExecutionFailed {
		t.Error("log request failure lost", f)
	}
}
