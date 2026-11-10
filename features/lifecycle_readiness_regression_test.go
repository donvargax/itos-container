package features

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type lifecycleObservationTransport func(*http.Request) (*http.Response, error)

func (f lifecycleObservationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Only temporary file observations and an in-memory HTTP transport. No product,
// approval command, native worker or engine socket enters this fixture.
func lifecycleReadinessWorld(t *testing.T) *world {
	t.Helper()
	dir := t.TempDir()
	w := &world{
		doctor:   &doctorWorld{checkout: dir, imageID: "image", env: map[string]string{"XDG_STATE_HOME": dir}},
		approval: &approvalWorld{project: "project"}, session: &sessionWorld{token: "token"},
		recovery:  &recoveryNative{},
		lifecycle: &lifecycleNative{mode: "handle", run: &recoveryProcess{exit: -1, started: time.Now(), done: make(chan struct{})}},
	}
	a := &offlineAPI{w: w, created: []string{strings.Repeat("a", 64)}}
	w.offline = &offlineRuntime{api: a}
	w.doctor.real = &doctorReal{client: &http.Client{Transport: lifecycleObservationTransport(func(*http.Request) (*http.Response, error) {
		body := approvalJSON(map[string]any{"Id": a.created[0], "Name": "/itos-container-token", "Image": "image",
			"Config": map[string]any{"Labels": w.sessionLabels()}, "State": map[string]any{"Running": true}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}}
	if err := os.WriteFile(filepath.Join(dir, "slice-12-ready"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(dir, "itos-container", "snapshots", "project")
	if err := os.MkdirAll(snapshot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "1.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestLifecycleReadinessBarrier(t *testing.T) {
	for _, tc := range []struct {
		name, first, last string
		delay, age        time.Duration
		want              bool
	}{
		{name: "already complete", first: "slice-12:ready\n", want: true},
		{name: "complete line after diagnostic", first: "diagnostic\nslice-12:ready\n", want: true},
		{name: "delayed frame", last: "slice-12:ready\n", delay: time.Second, want: true},
		{name: "split frame", first: "slice-12:rea", last: "dy\n", delay: time.Second, want: true},
		{name: "missing newline", first: "slice-12:ready"},
		{name: "wrong frame", first: "wrong-slice-12:ready\n"},
		{name: "missing frame"},
		{name: "late frame", last: "slice-12:ready\n", delay: 36 * time.Second},
		{name: "progress cannot restart allowance", first: "slice-12:", last: "rea", delay: time.Second, age: 33 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := lifecycleReadinessWorld(t)
				p := w.lifecycle.run
				p.started = p.started.Add(-tc.age)
				_, _ = p.out.Write([]byte(tc.first))
				if tc.last != "" {
					written := make(chan struct{})
					defer func() { <-written }()
					go func() { defer close(written); time.Sleep(tc.delay); _, _ = p.out.Write([]byte(tc.last)) }()
				}
				got := w.lifecycleReady()
				elapsed := time.Since(p.started)
				wantElapsed := 35 * time.Second
				if tc.want {
					wantElapsed = tc.age + tc.delay
				}
				latest := wantElapsed
				if tc.want && tc.delay > 0 {
					latest += 10 * time.Millisecond // one observation poll after publication
				}
				if got != tc.want || elapsed < wantElapsed || elapsed > latest {
					t.Errorf("readiness=%t at %s, want %t at %s; file/running alone cannot authorize an action", got, elapsed, tc.want, wantElapsed)
				}
				if got != !w.lifecycle.ready.IsZero() {
					t.Error("readiness timestamp does not describe complete evidence")
				}
			})
		})
	}
	t.Run("early exit", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w := lifecycleReadinessWorld(t)
			p := w.lifecycle.run
			defer func() { <-p.done }()
			go func() { time.Sleep(time.Second); p.exit = 75; p.finished = time.Now(); close(p.done) }()
			if w.lifecycleReady() || time.Since(p.started) != time.Second {
				t.Error("early exit did not end the missing-frame wait promptly as failure")
			}
		})
	})
	t.Run("observer failure", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w := lifecycleReadinessWorld(t)
			failure := errors.New("synthetic observer failure")
			w.doctor.real.client.Transport = lifecycleObservationTransport(func(*http.Request) (*http.Response, error) { return nil, failure })
			if w.lifecycleReady() || time.Since(w.lifecycle.run.started) != 0 || !errors.Is(w.lifecycle.err, failure) {
				t.Errorf("observer failure lost or retried until deadline: %v", w.lifecycle.err)
			}
		})
	})
	t.Run("observer receives remaining deadline", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w := lifecycleReadinessWorld(t)
			p := w.lifecycle.run
			p.started = p.started.Add(-34 * time.Second)
			w.doctor.real.client.Transport = lifecycleObservationTransport(func(r *http.Request) (*http.Response, error) {
				<-r.Context().Done()
				return nil, r.Context().Err()
			})
			if w.lifecycleReady() || time.Since(p.started) != 35*time.Second {
				t.Errorf("observer I/O escaped original allowance: %s", time.Since(p.started))
			}
		})
	})
	t.Run("controller exit cancels observer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w := lifecycleReadinessWorld(t)
			p := w.lifecycle.run
			w.doctor.real.client.Transport = lifecycleObservationTransport(func(r *http.Request) (*http.Response, error) {
				go func() { time.Sleep(time.Second); p.finished = time.Now(); p.exit = 75; close(p.done) }()
				<-r.Context().Done()
				return nil, r.Context().Err()
			})
			if w.lifecycleReady() || time.Since(p.started) != time.Second {
				t.Error("pending observer hid controller exit")
			}
		})
	})
	t.Run("recorder contention cannot overrun deadline", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w := lifecycleReadinessWorld(t)
			w.offline.api.mu.Lock()
			defer w.offline.api.mu.Unlock()
			_, _ = w.lifecycle.run.out.Write([]byte(lifecycleReadyFrame))
			if w.lifecycleReady() || time.Since(w.lifecycle.run.started) != 35*time.Second {
				t.Fatal("recorder lock stranded readiness beyond its deadline")
			}
		})
	})
	for _, missing := range []string{"ready file", "running identity", "recorder"} {
		t.Run(missing, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := lifecycleReadinessWorld(t)
				_, _ = w.lifecycle.run.out.Write([]byte(lifecycleReadyFrame))
				wantElapsed := 35 * time.Second
				switch missing {
				case "ready file":
					if err := os.Remove(filepath.Join(w.doctor.checkout, "slice-12-ready")); err != nil {
						t.Fatal(err)
					}
				case "running identity":
					w.offline.api.created = nil
				case "recorder":
					w.offline.api.problem = errors.New("synthetic recorder failure")
					wantElapsed = 0
				}
				if w.lifecycleReady() || time.Since(w.lifecycle.run.started) != wantElapsed {
					t.Error("captured frame bypassed another required observation")
				}
				lifecycleRequireDiagnostic(t, w.lifecycle.err, missing, "mode=", "condition=", "elapsed=", "limit=35s")
			})
		})
	}
	for _, mode := range []string{"handle", "disconnect", "healthy control"} {
		for _, complete := range []bool{false, true} {
			t.Run(mode+map[bool]string{true: " delayed", false: " missing"}[complete], func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					w := lifecycleReadinessWorld(t)
					w.lifecycle.mode = mode
					if complete {
						go func() { time.Sleep(time.Second); _, _ = w.lifecycle.run.out.Write([]byte(lifecycleReadyFrame)) }()
					}
					called := false
					ok := w.lifecycleReadyAction(func() {
						called = true
						if w.lifecycle.ready.IsZero() || time.Since(w.lifecycle.run.started) < time.Second {
							t.Error("signal/disconnect preceded complete readiness")
						}
					})
					if ok != complete || called != complete {
						t.Error("missing readiness authorized an action or complete readiness blocked it")
					}
				})
			})
		}
	}
	t.Run("unread output", testLifecycleReadinessPipes)
}

func testLifecycleReadinessPipes(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native pipe-capacity observation requires Linux")
	}
	for _, stdout := range []bool{false, true} {
		t.Run(map[bool]string{true: "stdout", false: "stderr"}[stdout], func(t *testing.T) {
			w := lifecycleReadinessWorld(t)
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = read.Close(); _ = write.Close() }()
			w.lifecycle.pipe = read
			w.lifecycle.condition = "unread workload stderr"
			prelude := ""
			if stdout {
				w.lifecycle.condition, prelude = "unread workload stdout", lifecycleReadyFrame
			} else {
				_, _ = w.lifecycle.run.out.Write([]byte(lifecycleReadyFrame))
			}
			const retained = "deliberately unread bytes"
			if _, err := write.Write([]byte(prelude + retained)); err != nil {
				t.Fatal(err)
			}
			if !w.lifecycleReady() || string(w.lifecycle.run.out.bytes()) != lifecycleReadyFrame {
				t.Fatalf("pipe path lost readiness: %v", w.lifecycle.err)
			}
			_, available, err, readErr := lifecyclePipeSize(read)
			if err != nil || readErr != nil || available != len(retained) {
				t.Fatal("readiness drained deliberately unread output", available, err, readErr)
			}
		})
	}
}
