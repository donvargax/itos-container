package features

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
					go func() { time.Sleep(tc.delay); _, _ = p.out.Write([]byte(tc.last)) }()
				}
				got := w.lifecycleReady()
				elapsed := time.Since(p.started)
				wantElapsed := 35 * time.Second
				if tc.want {
					wantElapsed = tc.age + tc.delay
				}
				if got != tc.want || elapsed != wantElapsed {
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
}
