package docker

import (
	"encoding/binary"
	"encoding/json"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
)

type egressAPI struct {
	replyStatus     int
	malformed       bool
	shortID         bool
	mu              sync.Mutex
	networks        map[string]map[string]any
	containers      map[string]*container.InspectResponse
	requests        []string
	networkBodies   []map[string]any
	containerBodies []egressCreate
	defect          func(*container.InspectResponse)
	networkDefect   func(map[string]any)
	imageDefect     func(map[string]any)
	logs            []byte
	logQueries      []string
	logFailure      bool
}
type egressCreate struct {
	container.Config
	HostConfig       *container.HostConfig
	NetworkingConfig *network.NetworkingConfig
}

func logFrame(stream byte, data string) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(data)))
	return append(h, []byte(data)...)
}
func egressSDKFixture(t *testing.T) (Engine, sp.Binding, *egressAPI, *st.Readiness) {
	t.Helper()
	a := &egressAPI{networks: map[string]map[string]any{}, containers: map[string]*container.InspectResponse{}, logs: append(logFrame(2, st.ProxyStderr), logFrame(1, st.ProxyStdout)...)}
	e, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
		a.mu.Lock()
		defer a.mu.Unlock()
		path := strings.TrimPrefix(r.URL.Path, "/v1.56")
		a.requests = append(a.requests, r.Method+" "+path)
		w.Header().Set("Content-Type", "application/json")
		if a.replyStatus != 0 {
			w.WriteHeader(a.replyStatus)
			return
		}
		if a.malformed {
			_, _ = w.Write([]byte("{broken"))
			return
		}
		switch {
		case path == "/info":
			_ = json.NewEncoder(w).Encode(map[string]string{"ID": b.EngineID})
		case strings.HasPrefix(path, "/images/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/json")
			v := map[string]any{"Id": id, "Config": map[string]any{"Env": []string{"IMAGE=retained", "HOME=/root"}}}
			if a.imageDefect != nil {
				a.imageDefect(v)
			}
			_ = json.NewEncoder(w).Encode(v)
		case path == "/networks/create":
			var v map[string]any
			_ = json.NewDecoder(r.Body).Decode(&v)
			a.networkBodies = append(a.networkBodies, v)
			id := strings.Repeat("a", 64)
			if len(a.networkBodies) > 1 {
				id = strings.Repeat("b", 64)
			}
			data, _ := json.Marshal(v)
			var observed map[string]any
			_ = json.Unmarshal(data, &observed)
			observed["Id"], observed["Scope"], observed["Containers"] = id, "local", map[string]any{}
			if observed["Internal"] != true {
				observed["IPAM"] = map[string]any{"Config": []any{map[string]any{"Gateway": "172.20.0.1"}}}
			}
			a.networks[id] = observed
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": func() string {
				if a.shortID {
					return "short"
				}
				return id
			}()})
		case strings.HasPrefix(path, "/networks/"):
			target := strings.TrimPrefix(path, "/networks/")
			v := a.networks[target]
			if v == nil {
				for _, candidate := range a.networks {
					if candidate["Name"] == target {
						v = candidate
						break
					}
				}
			}
			if v == nil {
				w.WriteHeader(404)
				return
			}
			if r.Method == "DELETE" {
				delete(a.networks, target)
				w.WriteHeader(204)
				return
			}
			data, _ := json.Marshal(v)
			var copy map[string]any
			_ = json.Unmarshal(data, &copy)
			if a.networkDefect != nil {
				a.networkDefect(copy)
			}
			_ = json.NewEncoder(w).Encode(copy)
		case path == "/containers/create":
			var v egressCreate
			_ = json.NewDecoder(r.Body).Decode(&v)
			a.containerBodies = append(a.containerBodies, v)
			id := strings.Repeat("c", 64)
			if len(a.containerBodies) > 1 {
				id = strings.Repeat("d", 64)
			}
			nets := map[string]*network.EndpointSettings{}
			if v.NetworkingConfig != nil {
				nets = v.NetworkingConfig.EndpointsConfig
			}
			observed := &container.InspectResponse{ID: id, Name: "/" + r.URL.Query().Get("name"), Image: v.Image, Config: &v.Config, HostConfig: v.HostConfig, State: &container.State{Status: "created"}, NetworkSettings: &container.NetworkSettings{Networks: nets}}
			if v.HostConfig != nil {
				for _, m := range v.HostConfig.Mounts {
					point := container.MountPoint{Type: m.Type, Source: m.Source, Destination: m.Target, RW: !m.ReadOnly}
					if m.Type == mount.TypeVolume {
						point.Name = m.Source
						point.Driver = "local"
					}
					observed.Mounts = append(observed.Mounts, point)
				}
			}
			a.containers[id] = observed
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": func() string {
				if a.shortID {
					return "short"
				}
				return id
			}()})
		case strings.HasPrefix(path, "/containers/"):
			parts := strings.Split(strings.TrimPrefix(path, "/containers/"), "/")
			id := parts[0]
			v := a.containers[id]
			if v == nil {
				for _, candidate := range a.containers {
					if candidate.Name == "/"+id {
						v = candidate
						break
					}
				}
			}
			if v == nil {
				w.WriteHeader(404)
				return
			}
			if r.Method == "DELETE" {
				delete(a.containers, id)
				for _, n := range a.networks {
					delete(n["Containers"].(map[string]any), id)
				}
				w.WriteHeader(204)
				return
			}
			switch parts[1] {
			case "json":
				data, _ := json.Marshal(v)
				var copy container.InspectResponse
				_ = json.Unmarshal(data, &copy)
				if a.defect != nil {
					a.defect(&copy)
				}
				_ = json.NewEncoder(w).Encode(copy)
			case "start":
				v.State = &container.State{Status: "running", Running: true, Pid: 123, StartedAt: "2026-11-09T12:00:00Z"}
				for name, ep := range v.NetworkSettings.Networks {
					if ep == nil {
						continue
					}
					ep.EndpointID = strings.Repeat("f", 64)
					ep.IPAddress = netip.MustParseAddr("172.20.0.2")
					for _, n := range a.networks {
						if n["Name"] == name {
							n["Containers"].(map[string]any)[id] = map[string]string{"Name": v.Name}
						}
					}
				}
				w.WriteHeader(204)
			case "kill":
				v.State = &container.State{Status: "exited", StartedAt: "2026-11-09T12:00:00Z"}
				w.WriteHeader(204)
			case "wait":
				_ = json.NewEncoder(w).Encode(map[string]any{"StatusCode": 137})
			case "logs":
				a.logQueries = append(a.logQueries, r.URL.RawQuery)
				if a.logFailure {
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
				_, _ = w.Write(a.logs)
			default:
				t.Errorf("unexpected container request %s", path)
				w.WriteHeader(400)
			}
		default:
			t.Errorf("unexpected request %s", path)
			w.WriteHeader(400)
		}
	})
	e.Executable = "/usr/local/bin/itos-container"
	b = st.EgressBinding(b)
	b.Environment = `{"APP":"literal","HTTP_PROXY":"wrong"}`
	b.Workspace = "/workspace/subdir"
	return e, b, a, pin
}
func createEgressFixture(t *testing.T, e Engine, b sp.Binding) (sp.RunConnection, sp.Record) {
	t.Helper()
	c, f := e.OpenRun(t.Context(), b)
	if f != "" {
		t.Fatal(f)
	}
	t.Cleanup(c.Close)
	r := st.EgressRecords(b)[1]
	for _, kind := range []sp.NetworkKind{sp.InternalNetwork, sp.EgressNetwork} {
		resource := &r.Egress.Internal
		if kind == sp.EgressNetwork {
			resource = &r.Egress.Network
		}
		resource.Stage = "intent"
		id, f := c.CreateNetwork(t.Context(), r, kind)
		if f != "" {
			t.Fatalf("expected network creation: %s", f)
		}
		resource.Stage, resource.ID = "owned", id
	}
	r.Egress.Proxy.Stage = "intent"
	id, f := c.CreateProxy(t.Context(), r)
	if f != "" {
		t.Fatalf("expected proxy creation: %s", f)
	}
	r.Egress.Proxy.Stage, r.Egress.Proxy.ID = "owned", id
	return c, r
}
