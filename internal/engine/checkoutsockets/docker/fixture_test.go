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

// answeredBy is a /info body from the daemon whose identity fields are the given
// raw JSON members, keeping runc registered as the default runtime. The pin the
// adapter validates reports engine-A, linux and x86_64, which info answers with.
func answeredBy(identity, runtimes string) string {
	return `{` + identity + `,"DefaultRuntime":"runc","Runtimes":` + runtimes + `}`
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
// the negotiated API version, and records how the adapter treats it. Like the
// real session, it keeps the lifetime it was established with, and answers
// requests made through a ForContext view on that view's context instead.
type pin struct {
	endpoint                    string
	version                     rp.Version
	info                        rp.Info
	versionFailure, infoFailure rp.Failure
	changed                     atomic.Bool
	closes                      atomic.Int32
	// lifetime bounds a request made on the session itself.
	lifetime context.Context
	// stall names the inspection, "version" or "info", that waits for the
	// context it runs on to end, announcing itself on arrived.
	stall   string
	arrived chan struct{}
}

func (p *pin) Endpoint() string { return p.endpoint }
func (p *pin) Deadline() time.Time {
	deadline, _ := p.lifetime.Deadline()
	return deadline
}

func (p *pin) Image(string) (rp.Image, rp.Failure) { return rp.Image{}, rp.ImageInspect }
func (p *pin) Unchanged() bool                     { return !p.changed.Load() }
func (p *pin) Close()                              { p.closes.Add(1) }

func (p *pin) Version() (rp.Version, rp.Failure) { return p.versionOn(p.lifetime) }
func (p *pin) Info() (rp.Info, rp.Failure)       { return p.infoOn(p.lifetime) }

func (p *pin) versionOn(ctx context.Context) (rp.Version, rp.Failure) {
	if f := p.hold(ctx, "version"); f != rp.None {
		return rp.Version{}, f
	}
	return p.version, p.versionFailure
}

func (p *pin) infoOn(ctx context.Context) (rp.Info, rp.Failure) {
	if f := p.hold(ctx, "info"); f != rp.None {
		return rp.Info{}, f
	}
	return p.info, p.infoFailure
}

func (p *pin) hold(ctx context.Context, inspection string) rp.Failure {
	if p.stall != inspection {
		return rp.None
	}
	select {
	case p.arrived <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return rp.EngineTimeout
}

// ForContext is a request view: it shares the pin and never closes it.
func (p *pin) ForContext(ctx context.Context) rp.Session { return view{p, ctx} }

type view struct {
	*pin
	ctx context.Context
}

func (v view) Deadline() time.Time {
	deadline, _ := v.ctx.Deadline()
	return deadline
}

func (v view) Version() (rp.Version, rp.Failure) { return v.versionOn(v.ctx) }
func (v view) Info() (rp.Info, rp.Failure)       { return v.infoOn(v.ctx) }

// plain is a session that cannot bind a request to a context.
type plain struct{ rp.Session }

// engine opens the same pin for every call and records what it was asked for.
type engine struct {
	pin                 *pin
	failure             rp.Failure
	plain               bool
	opens               int
	selection, checkout string
}

func (e *engine) OpenContext(_ context.Context, selection, checkout string) (rp.Session, rp.Failure) {
	e.opens++
	e.selection, e.checkout = selection, checkout
	if e.failure != rp.None {
		return nil, e.failure
	}
	if e.plain {
		return plain{e.pin}, rp.None
	}
	return e.pin, rp.None
}

func newEngine(endpoint string) *engine {
	healthy := rf.HealthySession()
	return &engine{pin: &pin{endpoint: endpoint, version: healthy.VersionData, info: healthy.InfoData, lifetime: context.Background(), arrived: make(chan struct{}, 2)}}
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
