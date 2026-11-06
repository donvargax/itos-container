package docker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/client"

	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func sdkFixture(t *testing.T, imageCondition ...string) (Engine, sp.Binding, *st.Readiness) {
	t.Helper()
	resource := map[string]any(nil)
	return sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
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
	})
}

func sdkHandlerFixture(t *testing.T, handler func(http.ResponseWriter, *http.Request, sp.Binding)) (Engine, sp.Binding, *st.Readiness) {
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
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, b)
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

func TestOpenRetainsPinUntilConnectionClose(t *testing.T) {
	engine, b, pin := sdkFixture(t)
	conn, failure := engine.Open(context.Background(), b)
	if failure != "" || conn == nil {
		t.Fatalf("open = %v, %s", conn, failure)
	}
	t.Cleanup(conn.Close)
	if pin.Closed {
		t.Fatal("successful open released the pinned engine identity")
	}
	if failure = conn.Check(context.Background()); failure != "" {
		t.Fatal("returned connection cannot verify its engine", failure)
	}
	conn.Close()
	if !pin.Closed {
		t.Fatal("connection close retained the engine pin")
	}
}

func TestOpenReleasesPinWhenEngineSetupFails(t *testing.T) {
	for _, tc := range []struct {
		fault     string
		want      sp.Outcome
		infoReads int
	}{
		{"healthy", "", 1},
		{"version unavailable", sp.Incomplete, 0},
		{"info unavailable", sp.Incomplete, 1},
		{"invalid client endpoint", sp.Incomplete, 1},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			b, _, _ := st.Fixture()
			pin := rf.HealthySession()
			pin.Address = b.Endpoint
			pin.InfoData.ID = b.EngineID
			switch tc.fault {
			case "version unavailable":
				pin.VersionFailure = rp.EngineInspect
			case "info unavailable":
				pin.InfoFailure = rp.EngineInspect
			case "invalid client endpoint":
				// Keep the binding and pin equal so this reaches SDK client
				// construction, which must reject a Unix endpoint with no path.
				b.Endpoint, pin.Address = "unix://", "unix://"
			}
			ready := &rf.Engine{Session: pin}
			engine := Engine{Readiness: ready, Selection: b.Endpoint}
			conn, failure := engine.Open(context.Background(), b)
			if conn != nil {
				t.Cleanup(conn.Close)
			}
			if failure != tc.want || (conn == nil) != (tc.want != "") {
				t.Fatalf("open = %v, %s; want %s", conn, failure, tc.want)
			}
			if ready.Opens != 1 || ready.Selection != b.Endpoint || ready.Root != b.Checkout.Root || ready.Budget <= 0 || ready.Budget > 10*time.Second {
				t.Fatal("engine setup lost its selected endpoint or bounded acquisition")
			}
			if pin.InfoReads != tc.infoReads {
				t.Fatalf("engine info reads = %d, want %d", pin.InfoReads, tc.infoReads)
			}
			if pin.Closed != (tc.want != "") {
				t.Fatal("engine setup released a successful pin or retained a failed pin")
			}
			if conn != nil {
				conn.Close()
				if !pin.Closed {
					t.Fatal("connection close retained the pin")
				}
			}
		})
	}
}

func TestDialRefusesReplacedSocket(t *testing.T) {
	engine, b, pin := sdkFixture(t)
	conn, failure := engine.Open(context.Background(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer conn.Close()
	if failure = conn.Check(context.Background()); failure != "" {
		t.Fatal("healthy socket control failed", failure)
	}
	// A socket can change after a method's first check but before transport dial.
	// Exercise that last guard directly, without substituting the SDK transport.
	pin.Current = false
	c := conn.(*connection)
	socket, err := c.transport.DialContext(context.Background(), "tcp", "ignored")
	if socket != nil {
		_ = socket.Close()
		t.Fatal("transport dialed a replaced socket")
	}
	if err == nil || err.Error() != "engine socket changed" {
		t.Fatalf("dial did not refuse the changed identity: %v", err)
	}
}

func TestCheckRequiresCurrentDaemonEvidence(t *testing.T) {
	for _, tc := range []struct {
		condition string
		want      sp.Outcome
	}{
		{"healthy", ""},
		{"socket replaced after reply", sp.Refused},
		{"unavailable", sp.Incomplete},
		{"malformed response", sp.Incomplete},
		{"different daemon", sp.Refused},
		{"missing daemon ID", sp.Refused},
		{"redirect", sp.Incomplete},
	} {
		t.Run(tc.condition, func(t *testing.T) {
			var requests atomic.Int32
			engine, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v1.56/info" {
					t.Errorf("unexpected engine request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				id := b.EngineID
				switch tc.condition {
				case "unavailable":
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				case "malformed response":
					_, _ = w.Write([]byte(`{"ID":`))
					return
				case "different daemon":
					id = "foreign"
				case "missing daemon ID":
					id = ""
				case "redirect":
					http.Redirect(w, r, "/redirect-target", http.StatusFound)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"ID": id})
			})
			conn, failure := engine.Open(context.Background(), b)
			if failure != "" {
				t.Fatal("engine setup failed before revalidation", failure)
			}
			defer conn.Close()
			if tc.condition == "socket replaced after reply" {
				replaceSocketOnReply(t, conn, pin, b.Endpoint, "")
			}
			if got := conn.Check(context.Background()); got != tc.want {
				t.Fatalf("check = %s, want %s", got, tc.want)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("check sent %d requests; want one info request without redirect or retry", got)
			}
		})
	}
}

func TestInspectRequiresCompleteContainerIdentity(t *testing.T) {
	for _, missing := range []string{"none", "Config", "Id", "Name", "Image", "unavailable", "malformed response", "socket replaced after reply"} {
		t.Run(missing, func(t *testing.T) {
			id, token := strings.Repeat("c", 64), strings.Repeat("1", 64)
			var requests atomic.Int32
			engine, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v1.56/containers/"+id+"/json" {
					t.Errorf("unexpected engine request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if missing == "unavailable" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if missing == "malformed response" {
					_, _ = w.Write([]byte(`{"Id":`))
					return
				}
				record := b.Intent(token)
				body := map[string]any{
					"Id": id, "Name": "/" + record.Name, "Image": b.ImageID,
					"Config": map[string]any{"Labels": map[string]string{
						"io.itos-container.intent": token, "io.itos-container.project": record.Project,
						"io.itos-container.fingerprint": record.Fingerprint,
					}},
					"State": map[string]any{"Status": "created", "Running": false, "Pid": 0, "StartedAt": ""},
				}
				delete(body, missing)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(body)
			})
			conn, failure := engine.Open(context.Background(), b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer conn.Close()
			if missing == "socket replaced after reply" {
				replaceSocketOnReply(t, conn, pin, b.Endpoint, "")
			}
			observed, status := conn.Inspect(context.Background(), id)
			if missing == "none" {
				want := sp.Observation{ID: id, Name: b.Intent(token).Name, ImageID: b.ImageID, Token: token,
					Project: b.Frozen.Project, Fingerprint: b.Frozen.Fingerprint, StatePresent: true, Status: "created"}
				if status != sp.Observed || observed != want {
					t.Fatalf("complete inspection = %+v, %v; want %+v", observed, status, want)
				}
			} else {
				want := sp.Unavailable
				if missing == "socket replaced after reply" {
					want = sp.Changed
				}
				if status != want || observed != (sp.Observation{}) {
					t.Fatalf("%s returned usable identity: %+v, %v", missing, observed, status)
				}
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("inspection sent %d requests, want one lookup by full ID", got)
			}
		})
	}
}

func TestCreateFailureDoesNotReturnUsableIdentity(t *testing.T) {
	for _, condition := range []string{"empty ID", "short ID", "malformed response", "unavailable", "socket replaced after create"} {
		t.Run(condition, func(t *testing.T) {
			var images, creates atomic.Int32
			engine, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1.56/images/"+b.ImageID+"/json":
					images.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": b.ImageID, "Config": map[string]any{}})
				case r.Method == http.MethodPost && r.URL.Path == "/v1.56/containers/create":
					creates.Add(1)
					id := strings.Repeat("c", 64)
					switch condition {
					case "empty ID":
						id = ""
					case "short ID":
						id = "abc"
					case "malformed response":
						_, _ = w.Write([]byte(`{"Id":`))
						return
					case "unavailable":
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]string{"Id": id})
				default:
					t.Errorf("unexpected engine request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			conn, failure := engine.Open(context.Background(), b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer conn.Close()
			if condition == "socket replaced after create" {
				replaceSocketOnReply(t, conn, pin, b.Endpoint, "/v1.56/containers/create")
			}
			if id, got := conn.Create(context.Background(), b.Intent(strings.Repeat("1", 64))); id != "" || got != sp.Uncertain {
				t.Fatalf("create returned a usable identity or lost uncertainty: %q, %s", id, got)
			}
			if images.Load() != 1 || creates.Load() != 1 {
				t.Fatalf("image inspections = %d, create requests = %d; want one each", images.Load(), creates.Load())
			}
		})
	}
}

func TestCallsRetainOriginalDeadlineAndEarlierCallerDeadline(t *testing.T) {
	engine, b, _ := sdkFixture(t)
	deadline := time.Now().Add(5 * time.Second)
	original, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	conn, failure := engine.Open(original, b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer conn.Close()
	c := conn.(*connection)
	for _, earlier := range []bool{false, true} {
		caller, cancelCaller := context.WithCancel(context.Background())
		want := deadline
		if earlier {
			cancelCaller()
			want = deadline.Add(-time.Second)
			caller, cancelCaller = context.WithDeadline(context.Background(), want)
		}
		call, end := c.call(caller)
		if got, ok := call.Deadline(); !ok || !got.Equal(want) {
			t.Fatalf("call deadline = %v, want %v", got, want)
		}
		end()
		if call.Err() != context.Canceled || caller.Err() != nil || original.Err() != nil {
			t.Fatal("ending a call failed to release it or canceled its parent")
		}
		cancelCaller()
	}
	// A fresh caller still cannot extend the original operation's budget.
	if failure = conn.Check(context.Background()); failure != "" {
		t.Fatal("bounded connection no longer usable", failure)
	}
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

// The fixture changes its pinned identity once the reply is received,
// on the caller's goroutine, before any subsequent request can be dispatched.
type replacedSocketTransport struct {
	base http.RoundTripper
	pin  *st.Readiness
	path string
}

func (r replacedSocketTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(request)
	if r.path == "" || request.URL.Path == r.path {
		r.pin.Current = false
	}
	return response, err
}

func replaceSocketOnReply(t *testing.T, conn sp.Connection, pin *st.Readiness, endpoint, path string) {
	t.Helper()
	c := conn.(*connection)
	_ = c.api.Close()
	api, err := client.New(client.WithHost(endpoint), client.WithAPIVersion("1.56"),
		client.WithHTTPClient(&http.Client{Transport: replacedSocketTransport{base: c.transport, pin: pin, path: path}}))
	if err != nil {
		t.Fatal(err)
	}
	c.api = api
}

func TestImageInspectionRevalidatesPinnedSocket(t *testing.T) {
	engine, b, pin := sdkFixture(t, "socket replacement")
	conn, failure := engine.Open(context.Background(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer conn.Close()
	replaceSocketOnReply(t, conn, pin, b.Endpoint, "")
	if id, result := conn.Create(context.Background(), b.Intent(strings.Repeat("1", 64))); id != "" || result != sp.Refused {
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

func TestForegroundDistinguishesMissingEnvironmentFromTransportAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		failure rp.Failure
		want    sp.Outcome
	}{
		{rp.EngineUnavailable, sp.EnvironmentMissing},
		{rp.EngineInspect, sp.Incomplete},
		{rp.EngineTimeout, sp.Incomplete},
		{rp.EngineChanged, sp.Refused},
		{rp.EngineRemote, sp.Refused},
		{rp.EngineSelection, sp.Refused},
		{rp.EngineUntrusted, sp.Refused},
	} {
		t.Run(string(tc.failure), func(t *testing.T) {
			readiness := &rf.Engine{Failure: tc.failure}
			engine := Engine{Readiness: readiness, Selection: "unix:///fixture-engine.sock"}
			binding := sp.Binding{Endpoint: engine.Selection}
			connection, failure := engine.OpenRun(context.Background(), binding)
			if connection != nil || failure != tc.want || readiness.Opens != 1 || readiness.Selection != engine.Selection {
				t.Fatalf("foreground opened a fallback or lost the cause: %v, %s, %+v", connection, failure, readiness)
			}
			// The distinct unstarted-only contract must not change with foreground reporting.
			want := tc.want
			if tc.failure == rp.EngineUnavailable {
				want = sp.Incomplete
			}
			if c, f := engine.Open(context.Background(), binding); c != nil || f != want {
				t.Fatalf("unstarted-only failure contract changed: %v, %s", c, f)
			}
		})
	}
}
