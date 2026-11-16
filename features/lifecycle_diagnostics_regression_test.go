package features

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestLifecycleBarrierDiagnostics(t *testing.T) {
	t.Run("deadline cancellation remains failure after collection", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w := lifecycleReadinessWorld(t)
			p := w.lifecycle.run
			guard := errors.New("synthetic harness guard error")
			p.cancel = func() { p.exit = -1; p.err = guard; p.finished = time.Now(); close(p.done) }
			w.lifecycle.condition = "stdin writer remains open"
			if w.lifecycleReady() {
				t.Fatal("missing frame passed readiness")
			}
			w.lifecycleAbortEarly()
			err := w.lifecycleResult(-1) // matching a harness-kill code is never success
			lifecycleRequireDiagnostic(t, err, "foreground frame", "elapsed=35s", "limit=35s", "handle", "stdin writer remains open",
				"exit -1", "harness cancelled after readiness expiry", "synthetic harness guard error", "journal empty")
			if !errors.Is(err, guard) {
				t.Error("collection lost the guard error identity")
			}
		})
	})
	t.Run("collected success and recorder failure", func(t *testing.T) {
		p := &recoveryProcess{exit: 17, started: time.Now(), done: make(chan struct{})}
		p.finished = p.started.Add(time.Second)
		close(p.done)
		a := &offlineAPI{}
		w := &world{lifecycle: &lifecycleNative{run: p}, offline: &offlineRuntime{api: a}}
		if err := w.lifecycleResult(17); err != nil {
			t.Fatal("collected healthy result was refused", err)
		}
		a.problem = errors.New("synthetic recorder observation failure")
		err := w.lifecycleResult(17)
		lifecycleRequireDiagnostic(t, err, "exit 17", "synthetic recorder observation failure", "journal empty")
		if !errors.Is(err, a.problem) {
			t.Error("recorder failure identity was lost")
		}
	})
	t.Run("early exit retains original error and startup evidence", func(t *testing.T) {
		origin := time.Now().Add(-2 * time.Second)
		barrier := errors.New("missing foreground frame")
		guard := errors.New("synthetic process error")
		p := &recoveryProcess{started: origin, finished: origin.Add(time.Second), exit: 75, err: guard, done: make(chan struct{})}
		_, _ = p.errout.Write([]byte("run.startup-timeout: start acknowledgement\n"))
		close(p.done)
		a := &offlineAPI{requests: []offlineRequest{{Method: "POST", Path: "/containers/owned/start", Status: 500,
			At: origin.Add(100 * time.Millisecond), Replied: origin.Add(900 * time.Millisecond), Diagnostic: "synthetic start failure"}}}
		w := &world{lifecycle: &lifecycleNative{mode: "handle", err: barrier, run: p}, offline: &offlineRuntime{api: a}}
		err := w.lifecycleResult(17)
		if !errors.Is(err, barrier) || !errors.Is(err, guard) {
			t.Errorf("lost original causes: %v", err)
		}
		lifecycleRequireDiagnostic(t, err, "missing foreground frame", "exit 75", "before readiness deadline", "run.startup-timeout",
			"POST", "/containers/owned/start", "HTTP 500", "100ms", "900ms", "synthetic start failure")
		// A failed independent healthy control reaches the main row's first Then
		// before the main invocation starts. Retain the control's full evidence.
		parent := &world{lifecycle: &lifecycleNative{control: w, err: err}}
		lifecycleRequireDiagnostic(t, parent.lifecycleResult(75), "healthy control", "exit 75", "run.startup-timeout", "HTTP 500", "/containers/owned/start")
	})
	t.Run("no process or journal", func(t *testing.T) {
		w := &world{lifecycle: &lifecycleNative{err: errors.New("barrier failed")}}
		lifecycleRequireDiagnostic(t, w.lifecycleResult(0), "barrier failed", "unstarted", "recorder unavailable")
	})
	t.Run("empty journal and pending process", func(t *testing.T) {
		w := &world{lifecycle: &lifecycleNative{run: &recoveryProcess{started: time.Now(), done: make(chan struct{})}}, offline: &offlineRuntime{api: &offlineAPI{}}}
		lifecycleRequireDiagnostic(t, w.lifecycleResult(0), "not collected", "journal empty")
	})
	t.Run("completed without start observation", func(t *testing.T) {
		p := &recoveryProcess{done: make(chan struct{})}
		close(p.done)
		w := &world{lifecycle: &lifecycleNative{run: p}}
		lifecycleRequireDiagnostic(t, w.lifecycleResult(0), "unstarted", "timing unavailable", "recorder unavailable")
	})
	t.Run("bounded ordered journal excludes sensitive fields", func(t *testing.T) {
		origin := time.Now()
		p := &recoveryProcess{exit: 75, started: origin, finished: origin.Add(time.Second), done: make(chan struct{})}
		close(p.done)
		_, _ = p.errout.Write([]byte(strings.Repeat("stderr\n", 3000)))
		secret := "SYNTHETIC_PRIVATE_FIELD"
		a := &offlineAPI{problem: errors.New("synthetic recorder failure")}
		for i := range 100 {
			a.requests = append(a.requests, offlineRequest{Method: "GET", Path: "/info", Query: secret,
				Body: map[string]any{"Env": secret}, Observation: map[string]any{"Config": secret}, Anchor: map[string]any{"secret": secret},
				Record: sessionRecord{Bytes: secret}, Status: 200, At: origin.Add(time.Duration(i) * time.Millisecond),
				Replied: origin.Add(time.Duration(i+1) * time.Millisecond)})
		}
		a.requests[40].Method, a.requests[40].Path, a.requests[40].Status = "POST", "/containers/owned/start", 403
		a.requests[40].Diagnostic = strings.Repeat("denied\n", 2000)
		a.requests[40].State = map[string]any{"Error": "synthetic state error", "Config": secret}
		a.requests[99].Path, a.requests[99].Replied = "/last-progress", time.Time{}
		w := &world{lifecycle: &lifecycleNative{err: errors.New("barrier failed"), run: p}, offline: &offlineRuntime{api: a}}
		err := w.lifecycleResult(0)
		lifecycleRequireDiagnostic(t, err, "exit 75", "synthetic recorder failure", "total=100", "omitted=", "truncated", "HTTP 403", "/last-progress", "reply=missing", "synthetic state error")
		if err != nil && (strings.Contains(err.Error(), secret) || len(err.Error()) > 20000) {
			t.Errorf("diagnostics exposed unrelated fields or were unbounded (%d bytes)", len(err.Error()))
		}
		if err != nil {
			text := err.Error()
			if strings.Index(text, "#41 ") > strings.Index(text, "#100 ") || !strings.Contains(text, "omitted=88") ||
				strings.Index(text, "exit 75") > strings.Index(text, "stderr=") {
				t.Error("journal lost order/counts or verbose details preceded exit classification")
			}
		}
	})
}

func lifecycleRequireDiagnostic(t *testing.T, err error, words ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("failed observation became success")
	}
	for _, word := range words {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("diagnostic omitted %q: %s", word, err)
		}
	}
}
