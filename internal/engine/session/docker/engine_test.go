package docker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"

	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func sdkFixture(t *testing.T, imageCondition ...string) (Engine, sp.Binding, *st.Readiness) {
	t.Helper()
	dir, err := os.MkdirTemp("", "slice-9-sdk-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "socket")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skip("Unix socket fixture unavailable")
	}
	b, _, _ := st.Fixture()
	b.Endpoint = "unix://" + path
	pin := &st.Readiness{EndpointValue: b.Endpoint, Current: true, VersionValue: rp.Version{Product: "Docker Engine - Community", OS: "linux", Architecture: "amd64", Minimum: "1.44", Maximum: "1.56", Negotiated: "1.56", SDKMinimum: "1.44", SDKMaximum: "1.56"}, InfoValue: rp.Info{ID: b.EngineID, OS: "linux", Architecture: "amd64", OperatingSystem: "Linux"}}
	resource := map[string]any(nil)
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.56")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case path == "/info":
			_ = json.NewEncoder(w).Encode(map[string]string{"ID": b.EngineID})
		case path == "/images/"+b.ImageID+"/json":
			image := map[string]any{"Id": b.ImageID, "Config": map[string]any{}}
			if len(imageCondition) != 0 {
				switch imageCondition[0] {
				case "declared volume":
					image["Config"] = map[string]any{"Volumes": map[string]any{"/data": map[string]any{}}}
				case "different image config ID":
					image["Id"] = "sha256:" + strings.Repeat("e", 64)
				case "missing Config":
					delete(image, "Config")
				case "unavailable":
					w.WriteHeader(503)
					return
				case "malformed response":
					_, _ = w.Write([]byte(`{"broken"`))
					return
				}
			}
			_ = json.NewEncoder(w).Encode(image)
		case path == "/containers/create":
			if len(imageCondition) != 0 {
				t.Error("create attempted without safe image evidence")
			}
			var body struct {
				Image, User     string
				Cmd, Entrypoint []string
				Labels          map[string]string
				HostConfig      struct {
					NetworkMode            string
					ReadonlyRootfs         bool
					CapDrop, SecurityOpt   []string
					PidsLimit              int64
					AutoRemove, Privileged bool
					Binds                  []string
				}
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Image != b.ImageID || body.User != b.UID+":"+b.GID || len(body.Cmd) != 1 || body.Cmd[0] != "/bin/true" || body.Entrypoint == nil || len(body.Entrypoint) != 0 || body.HostConfig.NetworkMode != "none" || !body.HostConfig.ReadonlyRootfs || body.HostConfig.PidsLimit != 4096 || len(body.HostConfig.CapDrop) != 1 || body.HostConfig.CapDrop[0] != "ALL" || len(body.HostConfig.SecurityOpt) != 1 || body.HostConfig.SecurityOpt[0] != "no-new-privileges" || body.HostConfig.AutoRemove || body.HostConfig.Privileged || len(body.HostConfig.Binds) != 0 {
				t.Error("invalid minimal create request")
				w.WriteHeader(400)
				return
			}
			id := strings.Repeat("c", 64)
			resource = map[string]any{"Id": id, "Name": "/" + r.URL.Query().Get("name"), "Image": b.ImageID, "Config": map[string]any{"Labels": body.Labels}, "State": map[string]any{"Status": "created", "Running": false, "Pid": 0, "StartedAt": ""}}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
		case r.Method == "DELETE":
			if r.URL.Query().Get("force") == "1" || r.URL.Query().Get("v") == "1" {
				t.Error("unsafe removal")
			}
			resource = nil
			w.WriteHeader(204)
		default:
			if resource == nil {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"message":"absent"}`))
			} else {
				_ = json.NewEncoder(w).Encode(resource)
			}
		}
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return Engine{Readiness: pin, Selection: b.Endpoint}, b, pin
}
func TestEngineContracts(t *testing.T) {
	t.Run("SDK", func(t *testing.T) {
		st.EngineContract(t, func() (sp.Engine, sp.Binding) { engine, b, _ := sdkFixture(t); return engine, b })
	})
	t.Run("memory", func(t *testing.T) {
		st.EngineContract(t, func() (sp.Engine, sp.Binding) { b, _, engine := st.Fixture(); return engine, b })
	})
}
func TestEngineImageContracts(t *testing.T) {
	t.Run("SDK", func(t *testing.T) {
		st.EngineImageContract(t, func(condition string) (sp.Engine, sp.Binding) {
			engine, b, _ := sdkFixture(t, condition)
			return engine, b
		})
	})
	t.Run("memory", func(t *testing.T) {
		st.EngineImageContract(t, func(condition string) (sp.Engine, sp.Binding) {
			b, _, engine := st.Fixture()
			engine.ImageInspection = condition
			return engine, b
		})
	})
}

// The fixture changes its pinned identity once the image reply is received,
// on the caller's goroutine, before any subsequent request can be dispatched.
type replacedImageTransport struct {
	base http.RoundTripper
	pin  *st.Readiness
}

func (r replacedImageTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(request)
	r.pin.Current = false
	return response, err
}

func TestImageInspectionRevalidatesPinnedSocket(t *testing.T) {
	engine, b, pin := sdkFixture(t, "socket replacement")
	conn, failure := engine.Open(context.Background(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	c := conn.(*connection)
	_ = c.api.Close()
	api, err := client.New(client.WithHost(b.Endpoint), client.WithAPIVersion("1.56"),
		client.WithHTTPClient(&http.Client{Transport: replacedImageTransport{base: c.transport, pin: pin}}))
	if err != nil {
		t.Fatal(err)
	}
	c.api = api
	defer c.Close()
	if id, result := c.Create(context.Background(), b.Intent(strings.Repeat("1", 64))); id != "" || result != sp.Refused {
		t.Fatalf("image inspection lost socket identity: %q %s", id, result)
	}
}

func TestSocketAndDeadlineRefusals(t *testing.T) {
	engine, b, pin := sdkFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, f := engine.Open(ctx, b); f != sp.Incomplete {
		t.Fatal(f)
	}
	pin.Failure = rp.EngineRemote
	if _, f := engine.Open(context.Background(), b); f != sp.Refused {
		t.Fatal(f)
	}
	pin.Failure = rp.EngineUnavailable
	if _, f := engine.Open(context.Background(), b); f != sp.Incomplete {
		t.Fatal(f)
	}
	pin.Failure = rp.None
	wrong := b
	wrong.Endpoint = "unix:///wrong"
	if _, f := engine.Open(context.Background(), wrong); f != sp.Refused {
		t.Fatal(f)
	}
	pin.InfoValue.ID = "foreign"
	if _, f := engine.Open(context.Background(), b); f != sp.Refused {
		t.Fatal(f)
	}
	pin.InfoValue.ID = b.EngineID
	pin.VersionValue.Product = "Podman"
	if _, f := engine.Open(context.Background(), b); f != sp.Refused {
		t.Fatal(f)
	}
	pin.VersionValue.Product = "Docker Engine - Community"
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, f := engine.Open(ctx, b)
	if f != "" {
		t.Fatal(f)
	}
	defer c.Close()
	if pin.Budget <= 0 || pin.Budget > time.Second {
		t.Fatal("caller budget reset", pin.Budget)
	}
	pin.Current = false
	if c.Check(ctx) != sp.Refused {
		t.Fatal("socket substitution accepted")
	}
	if _, status := c.Inspect(ctx, strings.Repeat("c", 64)); status != sp.Changed {
		t.Fatal(status)
	}
	if _, f = c.Create(ctx, b.Intent(strings.Repeat("1", 64))); f != sp.Refused {
		t.Fatal(f)
	}
	c.Remove(ctx, strings.Repeat("c", 64))
}
