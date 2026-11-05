package docker

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

func socketServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("local Unix inspection fixture requires Linux")
	}
	dir, err := os.MkdirTemp("/tmp", "slice-3-contract-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "engine.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return "unix://" + path
}

func fixtureHandler(t *testing.T, data *porttest.Session) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/v") {
			_, path, _ = strings.Cut(path[1:], "/")
			path = "/" + path
		}
		if r.URL.RawQuery != "" || (r.Method != "HEAD" && r.Method != "GET") {
			t.Errorf("non-inspection request: %s %s", r.Method, r.URL)
			w.WriteHeader(403)
			return
		}
		if path == "/_ping" {
			w.Header().Set("API-Version", "1.56")
			return
		}
		var body any
		switch {
		case path == "/version":
			if data.VersionFailure != port.None {
				w.WriteHeader(500)
				return
			}
			v := data.VersionData
			body = map[string]any{"Platform": map[string]string{"Name": v.Product}, "Os": v.OS, "Arch": v.Architecture, "ApiVersion": v.Maximum, "MinAPIVersion": v.Minimum}
		case path == "/info":
			if data.InfoFailure != port.None {
				w.WriteHeader(500)
				return
			}
			i := data.InfoData
			body = map[string]any{"ID": i.ID, "OSType": i.OS, "Architecture": i.Architecture, "OperatingSystem": i.OperatingSystem, "Warnings": []string{"synthetic-unreported"}}
		case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
			if data.ImageFailure != port.None {
				w.WriteHeader(500)
				return
			}
			image, ok := data.Images[strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/json")]
			if !ok {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"message":"absent"}`))
				return
			}
			body = map[string]any{"Id": image.ID, "Os": image.OS, "Architecture": image.Architecture, "RepoDigests": image.RepoDigests}
			if image.ConfigPresent {
				labels := map[string]string{"irrelevant": "synthetic-unreported"}
				if image.Label.Present {
					labels["devcontainer.metadata"] = image.Label.Text
				}
				body.(map[string]any)["Config"] = map[string]any{"User": image.User, "Labels": labels}
			}
		default:
			t.Errorf("unexpected request %s", path)
			w.WriteHeader(403)
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}
}

func TestEngineContract(t *testing.T) {
	porttest.EngineContract(t, func(data *porttest.Session) (port.Engine, string, string) {
		endpoint := socketServer(t, fixtureHandler(t, data))
		return Engine{}, endpoint, t.TempDir()
	})
}

func TestSelectionRefusesBeforeDial(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  port.Failure
	}{
		{"tcp://127.0.0.1:1", port.EngineRemote}, {"ssh://host", port.EngineRemote}, {"npipe:////./pipe/engine", port.EngineRemote},
		{"unix://relative.sock", port.EngineSelection}, {"unix:///tmp/x?", port.EngineSelection}, {"unix:///tmp/x#", port.EngineSelection},
		{"unix:///tmp/x?secret=yes", port.EngineSelection}, {"unix:///tmp/x#secret", port.EngineSelection}, {"unix://user:password@/tmp/x", port.EngineSelection},
		{"unix:///tmp/%zz", port.EngineSelection}, {"not a URI", port.EngineSelection}, {"unix:relative", port.EngineSelection},
	} {
		s, failure := (Engine{}).Open(tc.value, "/checkout", time.Second)
		if s != nil || failure != tc.want {
			t.Fatalf("selection %q = %v, %v", tc.value, s, failure)
		}
	}
}

func TestSocketAliasesAndReplacement(t *testing.T) {
	endpoint := socketServer(t, fixtureHandler(t, porttest.HealthySession()))
	path := strings.TrimPrefix(endpoint, "unix://")
	if _, failure := socketPath(endpoint, "relative"); failure != port.EngineSelection {
		t.Fatal(failure)
	}
	alias := filepath.Join(t.TempDir(), "alias.sock")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	s, failure := (Engine{}).Open("unix://"+alias, t.TempDir(), time.Second)
	if failure != port.None {
		t.Fatal(failure)
	}
	defer s.Close()
	if s.Endpoint() != endpoint || !s.Unchanged() {
		t.Fatal("alias not canonicalized")
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if s.Unchanged() {
		t.Fatal("socket removal not observed")
	}
	replacement, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = replacement.Close() }()
	if s.Unchanged() {
		t.Fatal("socket replacement not observed")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path+".old", alias); err != nil {
		t.Fatal(err)
	}
	if s.Unchanged() {
		t.Fatal("selection alias retargeting not observed")
	}
	if _, failure := socketPath(endpoint, filepath.Dir(path)); failure != port.EngineUntrusted {
		t.Fatal(failure)
	}
	if _, failure := socketPath(endpoint, path); failure != port.EngineUntrusted {
		t.Fatal(failure)
	}
}

func TestDefaultSelectionOnlyInspectsTheDefaultPath(t *testing.T) {
	// Only resolve paths; never connect to the host's default socket.
	root := t.TempDir()
	path, failure := socketPath("", root)
	explicit, expected := socketPath("unix:///var/run/docker.sock", root)
	if path != explicit || failure != expected {
		t.Fatal(path, failure, explicit, expected)
	}
}

func TestClientConstructionFailure(t *testing.T) {
	endpoint := socketServer(t, func(http.ResponseWriter, *http.Request) { t.Error("constructor failure must not connect") })
	s, failure := openEngine(endpoint, t.TempDir(), time.Second, func(...client.Opt) (*client.Client, error) { return nil, errors.New("synthetic constructor failure") })
	if s != nil || failure != port.EngineSelection {
		t.Fatal(s, failure)
	}
}

func TestSocketChangedDuringNegotiation(t *testing.T) {
	var path string
	endpoint := socketServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if err := os.Rename(path, path+".old"); err != nil {
			t.Error(err)
		}
		w.Header().Set("API-Version", "1.56")
	})
	path = strings.TrimPrefix(endpoint, "unix://")
	s, failure := (Engine{}).Open(endpoint, t.TempDir(), time.Second)
	if s != nil || failure != port.EngineChanged {
		t.Fatal(s, failure)
	}
}

func TestUnavailableSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "socket")
	for _, create := range []bool{false, true} {
		if create {
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if s, failure := (Engine{}).Open("unix://"+path, t.TempDir(), time.Second); s != nil || failure != port.EngineUnavailable {
			t.Fatal(s, failure)
		}
	}
}

func TestNegotiationFailureDoesNotRetry(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		version string
		wait    bool
		want    port.Failure
	}{
		{"http error", 500, "1.56", false, port.EngineInspect},
		{"unsupported api", 200, "1.24", false, port.EngineAPI},
		{"timeout", 200, "1.56", true, port.EngineTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			requests := 0
			endpoint := socketServer(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if test.wait {
					<-r.Context().Done()
					return
				}
				w.Header().Set("API-Version", test.version)
				w.WriteHeader(test.status)
			})
			s, failure := (Engine{}).Open(endpoint, t.TempDir(), 50*time.Millisecond)
			if s != nil || failure != test.want {
				t.Fatalf("open = %v, %v", s, failure)
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 1 {
				t.Fatalf("retried %d requests", requests)
			}
		})
	}
}
