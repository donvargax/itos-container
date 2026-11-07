package docker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type runAPI struct {
	mu            sync.Mutex
	resource      *container.InspectResponse
	attached      net.Conn
	defect        func(*container.InspectResponse)
	readInput     bool
	closeInput    bool
	input         []byte
	signals       []string
	signalTargets []string
}

func runSDKFixture(t *testing.T) (Engine, sp.Binding, *runAPI) {
	t.Helper()
	a := &runAPI{}
	engine, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
		a.mu.Lock()
		defer a.mu.Unlock()
		path := strings.TrimPrefix(r.URL.Path, "/v1.56")
		switch {
		case path == "/info":
			_ = json.NewEncoder(w).Encode(map[string]string{"ID": b.EngineID})
		case strings.HasPrefix(path, "/images/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": b.ImageID, "Config": map[string]any{"Env": []string{"IMAGE_DEFAULT=retained", "HOME=/root"}}})
		case path == "/containers/create":
			var body struct {
				container.Config
				HostConfig *container.HostConfig
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if body.HostConfig == nil || !safeHost(body.HostConfig, st.RunIntent(b).Anchor.Name) {
				t.Error("unsafe requested host configuration")
			}
			id := strings.Repeat("c", 64)
			a.resource = &container.InspectResponse{ID: id, Name: "/" + r.URL.Query().Get("name"), Image: b.ImageID, Config: &body.Config, HostConfig: body.HostConfig,
				State: &container.State{Status: "created"}, NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"none": {}}},
				Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: st.RunIntent(b).Anchor.Name, Driver: "local", Destination: "/workspace", RW: true}}}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
		case strings.HasSuffix(path, "/json"):
			if a.resource == nil {
				w.WriteHeader(404)
				return
			}
			// Mutate a copy so one faulty observation cannot change fixture truth.
			data, _ := json.Marshal(a.resource)
			var observed container.InspectResponse
			_ = json.Unmarshal(data, &observed)
			if a.defect != nil {
				a.defect(&observed)
			}
			_ = json.NewEncoder(w).Encode(observed)
		case strings.HasSuffix(path, "/attach"):
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			a.attached = conn
			_, _ = buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			_ = buf.Flush()
		case strings.HasSuffix(path, "/start"):
			if a.attached == nil {
				t.Error("start before attach")
				w.WriteHeader(400)
				return
			}
			a.resource.State = &container.State{Status: "exited", StartedAt: "2026-01-01T00:00:00Z"}
			if a.readInput {
				var err error
				a.input, err = io.ReadAll(a.attached)
				if err != nil {
					t.Error(err)
				}
			}
			writeFrame(a.attached, 1, []byte("stdout\x00\xff"))
			writeFrame(a.attached, 2, []byte("stderr\x00\xfe"))
			if a.closeInput {
				_ = a.attached.(*net.UnixConn).CloseWrite()
			} else {
				_ = a.attached.Close()
			}
			w.WriteHeader(204)
		case strings.HasSuffix(path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]any{"StatusCode": 7})
		case strings.HasSuffix(path, "/kill"):
			a.signals = append(a.signals, r.URL.Query().Get("signal"))
			a.signalTargets = append(a.signalTargets, strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/kill"))
			w.WriteHeader(204)
		case r.Method == "DELETE":
			if r.URL.Query().Get("force") != "1" || r.URL.Query().Get("v") == "1" {
				t.Error("incorrect owned removal")
			}
			a.resource = nil
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected SDK request: %s %s", r.Method, path)
			w.WriteHeader(400)
		}
	})
	b.Workspace, b.Environment = "/workspace/subdir", `{"APP":"literal;$HOST_ONLY"}`
	t.Cleanup(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.attached != nil {
			_ = a.attached.Close()
		}
	})
	return engine, b, a
}

func TestRunEngineContracts(t *testing.T) {
	t.Run("SDK", func(t *testing.T) {
		st.RunEngineContract(t, func() (sp.RunEngine, sp.Binding) { e, b, _ := runSDKFixture(t); return e, b })
	})
	t.Run("memory", func(t *testing.T) {
		st.RunEngineContract(t, func() (sp.RunEngine, sp.Binding) {
			b, _, e := st.Fixture()
			return &st.RunEngine{Engine: e, Exit: 7}, b
		})
	})
}

func TestForwardingFailureContracts(t *testing.T) {
	t.Run("SDK", func(t *testing.T) {
		st.ForwardingFailureContract(t, func() (sp.RunEngine, sp.Binding) { e, b, a := runSDKFixture(t); a.readInput = true; return e, b })
	})
	t.Run("memory", func(t *testing.T) {
		st.ForwardingFailureContract(t, func() (sp.RunEngine, sp.Binding) {
			b, _, e := st.Fixture()
			return &st.RunEngine{Engine: e, Output: []byte("out"), ErrorOutput: []byte("err")}, b
		})
	})
}

func TestStdinClosureContracts(t *testing.T) {
	t.Run("SDK", func(t *testing.T) {
		st.StdinClosureContract(t, func() (sp.RunEngine, sp.Binding, func()) {
			e, b, a := runSDKFixture(t)
			a.closeInput = true
			return e, b, func() {
				a.mu.Lock()
				defer a.mu.Unlock()
				if err := a.attached.Close(); err != nil {
					t.Error(err)
				}
			}
		})
	})
	t.Run("memory", func(t *testing.T) {
		st.StdinClosureContract(t, func() (sp.RunEngine, sp.Binding, func()) {
			b, _, e := st.Fixture()
			return &st.RunEngine{Engine: e, Exit: 7, StdinClosed: true}, b, func() {}
		})
	})
}

func TestSDKSignalUsesFullIDAndChecksConnection(t *testing.T) {
	e, b, a := runSDKFixture(t)
	ctx := context.Background()
	c, f := e.OpenRun(ctx, b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	id, f := c.CreateRun(ctx, st.RunIntent(b), []string{"probe"})
	if f != "" {
		t.Fatal(f)
	}
	for _, signal := range []string{"SIGINT", "SIGTERM", "SIGKILL"} {
		if f = c.Signal(ctx, id, signal); f != "" {
			t.Fatal(f)
		}
	}
	a.mu.Lock()
	got := slices.Clone(a.signals)
	targets := slices.Clone(a.signalTargets)
	a.mu.Unlock()
	if !slices.Equal(got, []string{"SIGINT", "SIGTERM", "SIGKILL"}) {
		t.Fatal(got)
	}
	if !slices.Equal(targets, []string{id, id, id}) {
		t.Fatal("signal targeted something other than the full owned worker ID", targets)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if c.Signal(canceled, id, "SIGTERM") != sp.ExecutionFailed {
		t.Fatal("canceled signal succeeded")
	}
}

func TestVerifyActualBoundary(t *testing.T) {
	cases := map[string]func(*container.InspectResponse){
		"network":            func(r *container.InspectResponse) { r.HostConfig.NetworkMode = "bridge" },
		"network attachment": func(r *container.InspectResponse) { r.NetworkSettings.Networks["bridge"] = &network.EndpointSettings{} },
		"published port": func(r *container.InspectResponse) {
			r.NetworkSettings.Ports = network.PortMap{network.MustParsePort("8080/tcp"): {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "18080"}}}
		},
		"root":            func(r *container.InspectResponse) { r.Config.User = "0:0" },
		"gid":             func(r *container.InspectResponse) { r.Config.User = "1000:0" },
		"group":           func(r *container.InspectResponse) { r.HostConfig.GroupAdd = []string{"0"} },
		"root filesystem": func(r *container.InspectResponse) { r.HostConfig.ReadonlyRootfs = false },
		"bind":            func(r *container.InspectResponse) { r.HostConfig.Binds = []string{"/:/host"} },
		"source":          func(r *container.InspectResponse) { r.HostConfig.Mounts[0].Source = "/host" },
		"anchor subpath":  func(r *container.InspectResponse) { r.HostConfig.Mounts[0].VolumeOptions.Subpath = "replacement" },
		"anchor copy":     func(r *container.InspectResponse) { r.HostConfig.Mounts[0].VolumeOptions.NoCopy = false },
		"propagation": func(r *container.InspectResponse) {
			r.HostConfig.Mounts[0].BindOptions = &mount.BindOptions{Propagation: "shared"}
		},
		"recursive": func(r *container.InspectResponse) {
			r.HostConfig.Mounts[0].BindOptions = &mount.BindOptions{NonRecursive: false}
		},
		"tmpfs":  func(r *container.InspectResponse) { r.HostConfig.Tmpfs = nil },
		"volume": func(r *container.InspectResponse) { r.Config.Volumes = map[string]struct{}{"/data": {}} },
		"materialized volume": func(r *container.InspectResponse) {
			r.Mounts = append(r.Mounts, container.MountPoint{Type: mount.TypeVolume, Destination: "/data"})
		},
		"absent bind": func(r *container.InspectResponse) { r.Mounts = nil },
		"duplicate bind": func(r *container.InspectResponse) {
			r.Mounts = append(r.Mounts, r.Mounts[0])
		},
		"duplicate tmpfs": func(r *container.InspectResponse) {
			tmpfs := container.MountPoint{Type: mount.TypeTmpfs, Destination: "/tmp"}
			r.Mounts = append(r.Mounts, tmpfs, tmpfs)
		},
		"drop":       func(r *container.InspectResponse) { r.HostConfig.CapDrop = nil },
		"capability": func(r *container.InspectResponse) { r.HostConfig.CapAdd = []string{"SYS_ADMIN"} },
		"privileged": func(r *container.InspectResponse) { r.HostConfig.Privileged = true },
		"nnp":        func(r *container.InspectResponse) { r.HostConfig.SecurityOpt = nil },
		"seccomp": func(r *container.InspectResponse) {
			r.HostConfig.SecurityOpt = append(r.HostConfig.SecurityOpt, "seccomp=unconfined")
		},
		"pid": func(r *container.InspectResponse) { r.HostConfig.PidMode = "host" },
		"ipc": func(r *container.InspectResponse) { r.HostConfig.IpcMode = "host" },
		"uts": func(r *container.InspectResponse) { r.HostConfig.UTSMode = "host" },
		"device": func(r *container.InspectResponse) {
			r.HostConfig.Devices = []container.DeviceMapping{{PathOnHost: "/dev/x"}}
		},
		"pids":             func(r *container.InspectResponse) { n := int64(4095); r.HostConfig.PidsLimit = &n },
		"restart":          func(r *container.InspectResponse) { r.HostConfig.RestartPolicy.Name = "always" },
		"autoremove":       func(r *container.InspectResponse) { r.HostConfig.AutoRemove = true },
		"entrypoint":       func(r *container.InspectResponse) { r.Config.Entrypoint = []string{"sh"} },
		"argv":             func(r *container.InspectResponse) { r.Config.Cmd = []string{"changed"} },
		"directory":        func(r *container.InspectResponse) { r.Config.WorkingDir = "/" },
		"environment":      func(r *container.InspectResponse) { r.Config.Env = append(r.Config.Env, "HOST_ONLY=leak") },
		"started":          func(r *container.InspectResponse) { r.State.Running = true },
		"missing security": func(r *container.InspectResponse) { r.HostConfig = nil },
		"missing network":  func(r *container.InspectResponse) { r.NetworkSettings = nil },
		"missing config":   func(r *container.InspectResponse) { r.Config = nil },
		"missing state":    func(r *container.InspectResponse) { r.State = nil },
		"wrong identity":   func(r *container.InspectResponse) { r.ID = strings.Repeat("e", 64) },
	}
	for name, defect := range cases {
		t.Run(name, func(t *testing.T) {
			e, b, a := runSDKFixture(t)
			ctx := context.Background()
			c, f := e.OpenRun(ctx, b)
			if f != "" {
				t.Fatal(f)
			}
			defer c.Close()
			argv := []string{"probe"}
			id, f := c.CreateRun(ctx, st.RunIntent(b), argv)
			if f != "" {
				t.Fatal(f)
			}
			if f = c.VerifyRun(ctx, id, argv); f != "" {
				t.Fatal("positive control", f)
			}
			if o, status := c.Inspect(ctx, id); status != sp.Observed || !o.AnchorExact {
				t.Fatal("missing exact anchor observation")
			}
			a.mu.Lock()
			a.defect = defect
			a.mu.Unlock()
			want := sp.IsolationUnverified
			if strings.HasPrefix(name, "missing") {
				want = sp.IsolationInspection
			}
			if name == "wrong identity" {
				want = sp.Refused
			}
			if f = c.VerifyRun(ctx, id, argv); f != want {
				t.Fatalf("got %s want %s", f, want)
			}
			if strings.HasPrefix(name, "anchor ") {
				if o, status := c.Inspect(ctx, id); status != sp.Observed || o.AnchorExact {
					t.Fatal("recovery accepted a different anchor mount")
				}
			}
		})
	}
}

func TestEnvironmentIsLiteralAndControllerOverridesHome(t *testing.T) {
	got := environment([]string{"PATH=/bin", "APP=image", "HOME=/root", "TMPDIR=/var/tmp"}, map[string]string{"APP": "$(touch marker);$HOST", "HOME": "ignored"})
	want := []string{"APP=$(touch marker);$HOST", "HOME=/tmp", "PATH=/bin", "TMPDIR=/tmp"}
	if !slices.Equal(got, want) {
		t.Fatal(got)
	}
}

func TestSDKStreamsKeepBinaryBytesSeparateAfterStdinEOF(t *testing.T) {
	e, b, a := runSDKFixture(t)
	a.readInput = true
	ctx := context.Background()
	c, f := e.OpenRun(ctx, b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	id, f := c.CreateRun(ctx, st.RunIntent(b), []string{"probe"})
	if f != "" {
		t.Fatal(f)
	}
	var out, errout strings.Builder
	input := "stdin\x00\xff\n"
	attached, f := c.Attach(ctx, id, sp.Streams{In: strings.NewReader(input), Out: &out, Err: &errout})
	if f != "" {
		t.Fatal(f)
	}
	defer attached.Close()
	if f = c.Start(ctx, id); f != "" {
		t.Fatal(f)
	}
	if _, f = c.Wait(ctx, id); f != "" {
		t.Fatal(f)
	}
	if f = attached.Drain(ctx); f != "" {
		t.Fatal(f)
	}
	a.mu.Lock()
	received := string(a.input)
	a.mu.Unlock()
	if received != input || out.String() != "stdout\x00\xff" || errout.String() != "stderr\x00\xfe" {
		t.Fatalf("streams changed: %q %q %q", received, out.String(), errout.String())
	}
}

func writeFrame(conn net.Conn, stream byte, data []byte) {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	_, _ = conn.Write(append(header, data...))
}
