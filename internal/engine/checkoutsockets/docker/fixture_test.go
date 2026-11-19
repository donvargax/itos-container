package docker

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/domain"
	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

var linuxAmd64 = port.Platform{OS: "linux", Architecture: "amd64"}

// daemon is a Unix-socket Docker API fixture. It answers only GET /v1.56/info
// and fails the test on any other request.
type daemon struct {
	endpoint string
	requests atomic.Int32
}

func serve(t *testing.T, respond http.HandlerFunc) *daemon {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("local Unix inspection fixture requires Linux")
	}
	dir, err := os.MkdirTemp("/tmp", "checkoutsockets-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "engine.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	d := &daemon{endpoint: "unix://" + path}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1.56/info" || r.URL.RawQuery != "" || r.ContentLength > 0 {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		d.requests.Add(1)
		respond(w, r)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return d
}

// info is a /info body that keeps runc registered and as the default runtime.
// Runtimes is the raw JSON of the whole field, or empty to omit it.
func info(runtimes string) string {
	body := `{"ID":"engine-A","OSType":"linux","Architecture":"x86_64","DefaultRuntime":"runc"`
	if runtimes != "" {
		body += `,"Runtimes":` + runtimes
	}
	return body + `}`
}

// withRunsc registers runc and the given runsc entry, varying only the latter.
func withRunsc(entry string) string {
	return info(`{"runc":{"path":"runc"},"runsc":` + entry + `}`)
}

func reply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// pin is the readiness session the caller validated: it fixes the endpoint and
// the negotiated API version, and records how the adapter treats it.
type pin struct {
	endpoint                    string
	version                     rp.Version
	info                        rp.Info
	versionFailure, infoFailure rp.Failure
	changed                     atomic.Bool
	closes                      atomic.Int32
}

func (p *pin) Endpoint() string                    { return p.endpoint }
func (p *pin) Version() (rp.Version, rp.Failure)   { return p.version, p.versionFailure }
func (p *pin) Info() (rp.Info, rp.Failure)         { return p.info, p.infoFailure }
func (p *pin) Image(string) (rp.Image, rp.Failure) { return rp.Image{}, rp.ImageInspect }
func (p *pin) Unchanged() bool                     { return !p.changed.Load() }
func (p *pin) Close()                              { p.closes.Add(1) }

// engine opens the same pin for every call and records what it was asked for.
type engine struct {
	pin                 *pin
	failure             rp.Failure
	opens               int
	selection, checkout string
}

func (e *engine) OpenContext(_ context.Context, selection, checkout string) (rp.Session, rp.Failure) {
	e.opens++
	e.selection, e.checkout = selection, checkout
	if e.failure != rp.None {
		return nil, e.failure
	}
	return e.pin, rp.None
}

func newEngine(endpoint string) *engine {
	healthy := rf.HealthySession()
	return &engine{pin: &pin{endpoint: endpoint, version: healthy.VersionData, info: healthy.InfoData}}
}

const (
	callerSelection = "unix:///run/caller-selected.sock"
	callerCheckout  = "/caller/checkout"
)

func adapter(e *engine) Runtimes {
	return Runtimes{Readiness: e, Selection: callerSelection, Checkout: callerCheckout}
}

// result is what the adapter reports: a registration or a typed failure that
// carries no registration, hence no selection.
type result struct {
	registration port.Registration
	failure      port.Failure
}

var (
	gvisor    = result{port.RunscRegistered, port.None}
	absent    = result{port.RunscAbsent, port.None}
	refused   = result{port.RegistrationUnknown, port.RuntimeRegistration}
	unread    = result{port.RegistrationUnknown, port.RuntimeInspect}
	cancelled = result{port.RegistrationUnknown, port.Cancelled}
)

func observe(ctx context.Context, e *engine) result {
	registration, failure := adapter(e).Runsc(ctx)
	return result{registration, failure}
}

// detect runs the full detector on a kernel at ABI 8, so only the daemon's
// registration decides between gVisor, neither and failure.
func detect(ctx context.Context, e *engine) port.Selection {
	kernel := &porttest.Kernel{ABI: port.ABI{Level: 8}}
	return domain.Detector{Kernel: kernel, Runtimes: adapter(e)}.Detect(ctx, linuxAmd64)
}

// expect serves one /info body and requires the adapter's answer and exactly
// one daemon request.
func expect(t *testing.T, name, body string, want result) {
	t.Helper()
	d := serve(t, reply(body))
	e := newEngine(d.endpoint)
	if got := observe(context.Background(), e); got != want {
		t.Errorf("%s: observed %+v, want %+v", name, got, want)
	}
	if d.requests.Load() != 1 {
		t.Errorf("%s: %d daemon requests, want 1", name, d.requests.Load())
	}
	if e.pin.closes.Load() != 1 {
		t.Errorf("%s: pin closed %d times, want 1", name, e.pin.closes.Load())
	}
}
