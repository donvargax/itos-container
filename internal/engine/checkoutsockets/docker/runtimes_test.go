package docker

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestRunscRegistration(t *testing.T) {
	t.Run("path_registration", func(t *testing.T) {
		for name, entry := range map[string]string{
			"bare name":                 `{"path":"runsc"}`,
			"absolute path":             `{"path":"/usr/local/bin/runsc"}`,
			"root directory":            `{"path":"/runsc"}`,
			"absolute with empty args":  `{"path":"/usr/local/bin/runsc","runtimeArgs":[]}`,
			"absolute with null args":   `{"path":"/usr/local/bin/runsc","runtimeArgs":null}`,
			"absolute with default uds": `{"path":"/usr/local/bin/runsc","runtimeArgs":["--host-uds=none"]}`,
			"bare name with status":     `{"path":"runsc","status":{"note":"reported"}}`,
			"empty runtime type":        `{"path":"runsc","runtimeType":""}`,
		} {
			expect(t, name, withRunsc(entry), gvisor)
		}
	})
	t.Run("shim_only_registration", func(t *testing.T) {
		expect(t, "no path and no status", withRunsc(`{}`), refused)
	})
	t.Run("contradictory_path", func(t *testing.T) {
		expect(t, "runc executable under the name runsc", withRunsc(`{"path":"/usr/bin/runc"}`), refused)
	})
	t.Run("socket_permitting_args", func(t *testing.T) {
		expect(t, "open", withRunsc(`{"path":"/usr/local/bin/runsc","runtimeArgs":["--host-uds=open"]}`), refused)
	})
	t.Run("unknown_args", func(t *testing.T) {
		for name, args := range map[string]string{
			"alone":                          `["--unknown-fixture-flag"]`,
			"default policy prepended":       `["--host-uds=none","--unknown-fixture-flag"]`,
			"default policy appended":        `["--unknown-fixture-flag","--host-uds=none"]`,
			"repeated default policy":        `["--host-uds=none","--host-uds=none"]`,
			"policy overridden later":        `["--host-uds=none","--host-uds=open"]`,
			"other policy value":             `["--host-uds=all"]`,
			"split flag and value":           `["--host-uds","none"]`,
			"different case":                 `["--HOST-UDS=none"]`,
			"empty argument":                 `[""]`,
			"default policy with whitespace": `["--host-uds=none "]`,
		} {
			expect(t, name, withRunsc(`{"path":"/usr/local/bin/runsc","runtimeArgs":`+args+`}`), refused)
		}
	})
	t.Run("unrecognized_path", func(t *testing.T) {
		for name, path := range map[string]string{
			"empty":                   ``,
			"relative":                `bin/runsc`,
			"dot relative":            `./runsc`,
			"trailing separator":      `/usr/bin/runsc/`,
			"parent component":        `/usr/bin/../bin/runsc`,
			"doubled separator":       `/usr//bin/runsc`,
			"upper case":              `RUNSC`,
			"capitalized final":       `/usr/bin/Runsc`,
			"trailing space":          `runsc `,
			"suffix":                  `/usr/bin/runsc2`,
			"prefix":                  `/usr/bin/gvisor-runsc`,
			"directory only":          `/`,
			"windows path":            `C:\\runsc`,
			"runsc as a directory":    `/usr/bin/runsc/runc`,
			"shim name":               `io.containerd.runsc.v1`,
			"home relative":           `~/runsc`,
			"current directory final": `/usr/bin/runsc/.`,
		} {
			expect(t, name, withRunsc(`{"path":"`+path+`"}`), refused)
		}
		expect(t, "null entry", withRunsc(`null`), refused)
		expect(t, "options without a path", withRunsc(`{"options":{"TypeUrl":"x"}}`), refused)
	})
	t.Run("runtime_type", func(t *testing.T) {
		for name, entry := range map[string]string{
			"runsc shim alone":             `{"runtimeType":"io.containerd.runsc.v1"}`,
			"runc shim alone":              `{"runtimeType":"io.containerd.runc.v2"}`,
			"runsc path with runc shim":    `{"path":"runsc","runtimeType":"io.containerd.runc.v2"}`,
			"absolute path with runc shim": `{"path":"/usr/local/bin/runsc","runtimeType":"io.containerd.runc.v2"}`,
			"runsc path with runsc shim":   `{"path":"runsc","runtimeType":"io.containerd.runsc.v1"}`,
			"runsc path with other shim":   `{"path":"runsc","runtimeType":"x"}`,
		} {
			expect(t, name, withRunsc(entry), refused)
		}
	})
	t.Run("absent_inventory", func(t *testing.T) {
		for name, runtimes := range map[string]string{
			"explicit empty map":      `{}`,
			"runc alone":              `{"runc":{"path":"runc"}}`,
			"capitalized lookalike":   `{"runc":{"path":"runc"},"Runsc":{"path":"/usr/local/bin/runsc"}}`,
			"upper case lookalike":    `{"runc":{"path":"runc"},"RUNSC":{"path":"runsc"}}`,
			"suffixed lookalike":      `{"runc":{"path":"runc"},"runsc2":{"path":"runsc"}}`,
			"padded lookalike":        `{"runc":{"path":"runc"}," runsc":{"path":"runsc"}}`,
			"shim name as the key":    `{"runc":{"path":"runc"},"io.containerd.runsc.v1":{"path":"runsc"}}`,
			"lookalike only, no runc": `{"gvisor":{"path":"/usr/local/bin/runsc"}}`,
		} {
			expect(t, name, info(runtimes), absent)
		}
	})
	t.Run("unusable_inventory", func(t *testing.T) {
		for name, body := range map[string]string{
			"omitted":         info(``),
			"null":            info(`null`),
			"array":           info(`[]`),
			"string":          info(`"runsc"`),
			"entry as string": info(`{"runsc":"runsc"}`),
			"path as number":  withRunsc(`{"path":7}`),
			"args as string":  withRunsc(`{"path":"runsc","runtimeArgs":"--host-uds=none"}`),
			"truncated":       `{"ID":"engine-A","Runtimes":{"runsc":{"path":"runsc"}`,
			"not JSON":        `not a response`,
			"empty body":      ``,
			"array response":  `[]`,
		} {
			expect(t, name, body, unread)
		}
		for status, text := range map[int]string{http.StatusInternalServerError: "failure", http.StatusNotFound: "absent", http.StatusForbidden: "denied", http.StatusBadGateway: "gateway"} {
			d := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"message":"` + text + `"}`))
			})
			if got := observe(context.Background(), newEngine(d.endpoint)); got != unread {
				t.Errorf("HTTP %d: observed %+v", status, got)
			}
		}
	})
	t.Run("redirect_not_followed", func(t *testing.T) {
		d := serve(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/v1.56/info?followed=yes", http.StatusFound)
		})
		if got := observe(context.Background(), newEngine(d.endpoint)); got != unread {
			t.Errorf("observed %+v", got)
		}
		if d.requests.Load() != 1 {
			t.Errorf("%d daemon requests, want 1", d.requests.Load())
		}
	})
	t.Run("cancelled_read", func(t *testing.T) {
		arrived, ended := make(chan struct{}), make(chan struct{})
		d := serve(t, func(_ http.ResponseWriter, r *http.Request) {
			close(arrived)
			<-r.Context().Done()
			close(ended)
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-arrived:
				cancel()
			case <-time.After(5 * time.Second):
			}
		}()
		e := newEngine(d.endpoint)
		if got := observe(ctx, e); got != cancelled {
			t.Errorf("observed %+v", got)
		}
		select {
		case <-ended:
		case <-time.After(5 * time.Second):
			t.Error("the request stayed pending after cancellation")
		}
		if e.pin.closes.Load() != 1 {
			t.Errorf("pin closed %d times", e.pin.closes.Load())
		}
	})
	t.Run("deadline_during_read", func(t *testing.T) {
		d := serve(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if got := observe(ctx, newEngine(d.endpoint)); got != cancelled {
			t.Errorf("observed %+v", got)
		}
	})
	t.Run("cancelled_before_any_request", func(t *testing.T) {
		d := serve(t, reply(withRunsc(`{"path":"runsc"}`)))
		e := newEngine(d.endpoint)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got, err := adapter(e).Runsc(ctx)
		if got != port.RegistrationUnknown || err != port.Cancelled || e.opens != 0 || d.requests.Load() != 0 {
			t.Errorf("observed %v, %v; opened %d times, %d requests", got, err, e.opens, d.requests.Load())
		}
	})
	t.Run("caller_selection", func(t *testing.T) {
		d := serve(t, reply(withRunsc(`{"path":"runsc"}`)))
		e := newEngine(d.endpoint)
		if got, f := adapter(e).Runsc(context.Background()); got != port.RunscRegistered || f != port.None {
			t.Errorf("observed %v, %v", got, f)
		}
		if e.selection != callerSelection || e.checkout != callerCheckout || e.opens != 1 {
			t.Errorf("opened %q, %q %d times", e.selection, e.checkout, e.opens)
		}
	})
	t.Run("validated_daemon", func(t *testing.T) {
		for name, mutate := range map[string]func(*engine){
			"readiness open failure":  func(e *engine) { e.failure = rp.EngineUnavailable },
			"untrusted socket":        func(e *engine) { e.failure = rp.EngineUntrusted },
			"version failure":         func(e *engine) { e.pin.versionFailure = rp.EngineInspect },
			"info failure":            func(e *engine) { e.pin.infoFailure = rp.EngineInspect },
			"foreign system":          func(e *engine) { e.pin.version.OS = "windows" },
			"foreign product":         func(e *engine) { e.pin.version.Product = "Docker Desktop" },
			"changed before the read": func(e *engine) { e.pin.changed.Store(true) },
			"unusable endpoint":       func(e *engine) { e.pin.endpoint = "tcp://%zz" },
			"no request contexts":     func(e *engine) { e.plain = true },
		} {
			d := serve(t, reply(withRunsc(`{"path":"runsc"}`)))
			e := newEngine(d.endpoint)
			if name != "unusable endpoint" {
				e.pin.endpoint = d.endpoint
			}
			mutate(e)
			got := observe(context.Background(), e)
			if got != unread {
				t.Errorf("%s: observed %+v", name, got)
			}
			if d.requests.Load() != 0 {
				t.Errorf("%s: %d daemon requests", name, d.requests.Load())
			}
			wantCloses := int32(1)
			if name == "readiness open failure" || name == "untrusted socket" {
				wantCloses = 0
			}
			if e.pin.closes.Load() != wantCloses {
				t.Errorf("%s: pin closed %d times, want %d", name, e.pin.closes.Load(), wantCloses)
			}
		}
	})
	t.Run("inspections_bound_to_the_call", func(t *testing.T) {
		// The session keeps the lifetime it was established with, which can
		// outlast the call, so each inspection must run on the call's context.
		for name, c := range map[string]struct {
			stall    string
			deadline bool
		}{
			"cancelled during the version request": {"version", false},
			"cancelled during the info request":    {"info", false},
			"deadline during the version request":  {"version", true},
			"deadline during the info request":     {"info", true},
		} {
			d := serve(t, reply(withRunsc(`{"path":"runsc"}`)))
			e := newEngine(d.endpoint)
			lifetime, release := context.WithCancel(context.Background())
			t.Cleanup(release)
			e.pin.lifetime, e.pin.stall = lifetime, c.stall
			ctx, cancel := context.WithCancel(context.Background())
			if c.deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			go func() {
				select {
				case <-e.pin.arrived:
					if !c.deadline {
						cancel()
					}
				case <-time.After(5 * time.Second):
				}
			}()
			done := make(chan result, 1)
			go func() { done <- observe(ctx, e) }()
			select {
			case got := <-done:
				if got != cancelled {
					t.Errorf("%s: observed %+v", name, got)
				}
				if e.pin.closes.Load() != 1 {
					t.Errorf("%s: pin closed %d times", name, e.pin.closes.Load())
				}
			case <-time.After(2 * time.Second):
				t.Errorf("%s: the inspection stayed pending after the call ended", name)
			}
			if d.requests.Load() != 0 {
				t.Errorf("%s: %d daemon requests", name, d.requests.Load())
			}
		}
	})
	t.Run("changed_during_the_read", func(t *testing.T) {
		var e *engine
		d := serve(t, func(w http.ResponseWriter, r *http.Request) {
			e.pin.changed.Store(true)
			reply(withRunsc(`{"path":"runsc"}`))(w, r)
		})
		e = newEngine(d.endpoint)
		if got := observe(context.Background(), e); got != unread {
			t.Errorf("observed %+v", got)
		}
		if d.requests.Load() != 1 {
			t.Errorf("%d daemon requests", d.requests.Load())
		}
	})
	t.Run("zero_value", func(t *testing.T) {
		if got, f := (Runtimes{}).Runsc(context.Background()); got != port.RegistrationUnknown || f != port.NoProbe {
			t.Errorf("observed %v, %v", got, f)
		}
	})
	t.Run("fresh_observation", func(t *testing.T) {
		var body atomic.Value
		body.Store(withRunsc(`{"path":"runsc"}`))
		d := serve(t, func(w http.ResponseWriter, r *http.Request) { reply(body.Load().(string))(w, r) })
		e := newEngine(d.endpoint)
		for i, step := range []struct {
			body string
			want result
		}{
			{withRunsc(`{"path":"runsc"}`), gvisor},
			{info(`{"runc":{"path":"runc"}}`), absent},
			{withRunsc(`{"path":"runsc","runtimeArgs":["--host-uds=open"]}`), refused},
			{withRunsc(`{"path":"/usr/local/bin/runsc"}`), gvisor},
		} {
			body.Store(step.body)
			if got := observe(context.Background(), e); got != step.want || int(d.requests.Load()) != i+1 {
				t.Errorf("observation %d = %+v after %d requests, want %+v", i, got, d.requests.Load(), step.want)
			}
		}
	})
	t.Run("detector_selection", func(t *testing.T) {
		for name, c := range map[string]struct {
			body string
			want port.Selection
		}{
			"path-based runsc":       {withRunsc(`{"path":"/usr/local/bin/runsc"}`), port.Selection{Backend: port.GVisor}},
			"shim-only registration": {withRunsc(`{}`), port.Selection{Failure: port.RuntimeRegistration}},
			"socket-permitting args": {withRunsc(`{"path":"runsc","runtimeArgs":["--host-uds=open"]}`), port.Selection{Failure: port.RuntimeRegistration}},
			"no runsc":               {info(`{"runc":{"path":"runc"}}`), port.Selection{Backend: port.Neither}},
			"missing inventory":      {info(``), port.Selection{Failure: port.RuntimeInspect}},
		} {
			d := serve(t, reply(c.body))
			if got := detect(context.Background(), newEngine(d.endpoint)); got != c.want {
				t.Errorf("%s: selection %+v, want %+v", name, got, c.want)
			}
		}
	})
	t.Run("shared_contract", func(t *testing.T) {
		porttest.RuntimesContract(t, func() porttest.RuntimesSubject {
			var mode atomic.Int32
			d := serve(t, func(w http.ResponseWriter, r *http.Request) {
				switch porttest.RuntimesReply(mode.Load()) {
				case porttest.ReplyRegistered:
					reply(withRunsc(`{"path":"/usr/local/bin/runsc"}`))(w, r)
				case porttest.ReplyAbsent:
					reply(info(`{"runc":{"path":"runc"}}`))(w, r)
				case porttest.ReplyUnrecognized:
					reply(withRunsc(`{}`))(w, r)
				default:
					w.WriteHeader(http.StatusInternalServerError)
				}
			})
			return porttest.RuntimesSubject{
				Runtimes: adapter(newEngine(d.endpoint)),
				Reply:    func(r porttest.RuntimesReply) { mode.Store(int32(r)) },
				Reads:    func() int { return int(d.requests.Load()) },
			}
		})
	})
}
