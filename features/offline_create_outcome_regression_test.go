package features

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// T-50: the recorder hears out every create it forwards. Each case drives serve,
// the handler every native fixture's recorder runs, against a disposable engine.
func TestOfflineRecorderRecordsAbandonedCreate(t *testing.T) {
	for _, kind := range []string{"plain", "recovery", "lifecycle"} {
		t.Run(kind+" fixture", func(t *testing.T) { testAbandonedCreate(t, kind) })
	}
	t.Run("recorder server closes during the exchange", testCreateServerClosed)
	t.Run("lifecycle disconnect during the exchange", testCreateDisconnected)
	t.Run("anchor create barrier", func(t *testing.T) {
		testCreateBarrier(t, "recovery", func(w *world) (<-chan struct{}, func()) {
			s := &anchorRecoveryWorld{mode: "create", ready: make(chan struct{}), release: make(chan struct{})}
			w.anchors = s
			return s.ready, func() { close(s.release) }
		})
	})
	t.Run("lifecycle create barrier", func(t *testing.T) {
		testCreateBarrier(t, "lifecycle", func(w *world) (<-chan struct{}, func()) {
			f := newLifecycleFaults("crash", "after accepted create before reply", 0)
			w.lifecycle.fault, w.offline.api.lifecycle = f, f
			return f.ready, f.releaseBarrier
		})
	})
	t.Run("engine never answers", func(t *testing.T) {
		for _, kind := range []string{"plain", "recovery", "lifecycle"} {
			t.Run(kind, func(t *testing.T) { synctest.Test(t, func(t *testing.T) { testCreateAllowance(t, kind, false) }) })
		}
		t.Run("connected controller", func(t *testing.T) { synctest.Test(t, func(t *testing.T) { testCreateAllowance(t, "plain", true) }) })
	})
	t.Run("step wait", func(t *testing.T) { synctest.Test(t, testCreateStepWait) })
	t.Run("engine-side failure with the controller connected", testCreateEngineFailure)
	t.Run("ordinary create", testCreateOrdinary)
}

// The accounting every abandoned create must end with, whatever made its
// controller leave: the worker listed under the request's identity, the journal
// entry holding the engine's status beside a cancelled controller side.
func requireAbandonedAccounting(t *testing.T, w *world, e *createEngine, token string) string {
	t.Helper()
	a := w.offline.api
	id := e.awaitWorker(t)
	if listed := createListed(a); !reflect.DeepEqual(listed, []string{id}) {
		t.Errorf("abandoned create's worker is not listed exactly once: %v", listed)
	}
	a.mu.Lock()
	identity := a.identities[id]
	a.mu.Unlock()
	labels := map[string]any{"io.itos-container.intent": token, "io.itos-container.project": w.approval.project, "io.itos-container.fingerprint": w.approval.fingerprint}
	if identity["Name"] != "/itos-container-"+token || !reflect.DeepEqual(identity["Labels"], labels) {
		t.Errorf("listed worker lacks the request's name and labels as its identity: %v", identity)
	}
	q := createEntry(t, a)
	if q.Status != http.StatusCreated {
		t.Errorf("journal entry status = %d, want the engine's 201", q.Status)
	}
	if !q.Cancelled || !q.Replied.IsZero() {
		t.Errorf("journal entry cancelled=%t replied=%t, want a cancelled controller side and no reply time", q.Cancelled, !q.Replied.IsZero())
	}
	if q.Forwarded.IsZero() || q.Answered.IsZero() || q.Forwarded.Before(q.At) || q.Answered.Before(q.Forwarded) {
		t.Errorf("journal entry lacks ordered forwarding and answer times: arrival=%v forwarded=%v answered=%v", q.At, q.Forwarded, q.Answered)
	}
	if q.Observation["Id"] != id {
		t.Errorf("journal entry lacks the recorder's own inspection of the worker: %v", q.Observation["Id"])
	}
	return id
}

func testAbandonedCreate(t *testing.T, kind string) {
	w, e := createRecorderWorld(t, kind)
	a := w.offline.api
	token := createTokens[0]
	c := startCreate(t, w, e, token)
	c.abandon(t)

	// The controller has left and the engine has not answered yet.
	problem := createProblem(a)
	if kind == "plain" {
		if problem != createAbandonedWords {
			t.Errorf("plain fixture problem at abandonment = %q, want %q", problem, createAbandonedWords)
		}
		if err := a.observeAfter(); err == nil || !strings.Contains(err.Error(), createAbandonedWords) {
			t.Errorf("observeAfter did not hand the abandonment to the first Then: %v", err)
		}
	} else if problem != "" {
		t.Errorf("%s fixture set a recorder problem for an abandoned request: %q", kind, problem)
	}
	if q := createEntry(t, a); !q.Cancelled || !q.Replied.IsZero() {
		t.Errorf("abandoned create is not journalled as cancelled without a reply: %+v", q)
	}
	if listed := createListed(a); len(listed) != 0 {
		t.Errorf("worker listed before the engine answered: %v", listed)
	}

	e.finish()
	a.awaitCreates()
	id := requireAbandonedAccounting(t, w, e, token)
	if kind != "plain" && createProblem(a) != "" {
		t.Errorf("%s fixture set a recorder problem for the late answer: %q", kind, createProblem(a))
	}

	// The worker the recorder let the product create is the product's to inspect
	// and remove by full ID; any other full ID stays refused and unforwarded.
	inspected := httptest.NewRecorder()
	a.serve(inspected, httptest.NewRequest("GET", "http://engine/containers/"+id+"/json", nil))
	if inspected.Code != http.StatusOK {
		t.Errorf("full-ID inspection of the listed worker = HTTP %d, want 200", inspected.Code)
	}
	before := len(e.journal())
	foreign := httptest.NewRecorder()
	a.serve(foreign, httptest.NewRequest("GET", "http://engine/containers/"+strings.Repeat("f", 64)+"/json", nil))
	if foreign.Code != http.StatusForbidden || len(e.journal()) != before {
		t.Errorf("another full ID = HTTP %d with %d forwarded requests, want 403 and none", foreign.Code, len(e.journal())-before)
	}
	removed := httptest.NewRecorder()
	a.serve(removed, httptest.NewRequest("DELETE", "http://engine/containers/"+id+"?force=true&v=false", nil))
	if removed.Code != http.StatusNoContent {
		t.Errorf("owned removal of the listed worker = HTTP %d, want 204", removed.Code)
	}
	if e.worker() != "" {
		t.Error("the worker is still in the engine after its owned removal")
	}
}

func testCreateServerClosed(t *testing.T) {
	w, e := createRecorderWorld(t, "plain")
	a := w.offline.api
	if err := w.startOfflineAPI(); err != nil {
		t.Fatal(err)
	}
	// startOfflineAPI made a recorder of its own; keep the fixture's engine.
	a = w.offline.api
	t.Cleanup(func() {
		_ = a.server.Close()
		e.finish()
		a.awaitCreates()
		w.closeDoctor()
	})
	transport := doctorTransport(strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://"))
	defer transport.CloseIdleConnections()
	token := createTokens[0]
	inner := createRequest(context.Background(), w, token)
	body, _ := io.ReadAll(inner.Body)
	request, err := http.NewRequestWithContext(t.Context(), "POST", "http://engine"+inner.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	failed := make(chan error, 1)
	go func() {
		response, err := (&http.Client{Transport: transport}).Do(request)
		if err == nil {
			_ = response.Body.Close()
		}
		failed <- err
	}()
	select {
	case <-e.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("create never reached the engine")
	}
	// closeOfflineRuntime ends the server this way before it cleans up.
	_ = a.server.Close()
	a.closeConnections()
	select {
	case err := <-failed:
		if err == nil {
			t.Error("a controller of a closed recorder server received a reply")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("closing the recorder server did not end the controller's request")
	}
	// The server notices its closed connection on a goroutine of its own. Let the
	// recorder account for the departure before the engine answers, as the cases
	// that drive serve directly do by waiting for the handler.
	deadline := time.Now().Add(5 * time.Second)
	for !createEntry(t, a).Cancelled {
		if time.Now().After(deadline) {
			t.Fatal("the recorder never noticed its closed server")
		}
		time.Sleep(time.Millisecond)
	}
	if problem := createProblem(a); problem != createAbandonedWords {
		t.Errorf("plain fixture problem after its server closed = %q, want %q", problem, createAbandonedWords)
	}
	e.finish()
	a.awaitCreates()
	requireAbandonedAccounting(t, w, e, token)
}

func testCreateDisconnected(t *testing.T) {
	w, e := createRecorderWorld(t, "lifecycle")
	a := w.offline.api
	token := createTokens[0]
	c := startCreate(t, w, e, token)
	// The controller's connection stays open: only the fault cancels its request.
	w.lifecycle.fault.disconnect()
	c.wait(t)
	if c.response.Code != http.StatusBadGateway || !strings.Contains(c.response.Body.String(), "offline fixture transport failed") {
		t.Errorf("connected controller read HTTP %d %q through the disconnect, want 502 and the transport-failure body", c.response.Code, c.response.Body.String())
	}
	e.finish()
	a.awaitCreates()
	requireAbandonedAccounting(t, w, e, token)
	if problem := createProblem(a); problem != "" {
		t.Errorf("lifecycle fixture set a recorder problem for a disconnected create: %q", problem)
	}
}

func testCreateBarrier(t *testing.T, kind string, arm func(*world) (<-chan struct{}, func())) {
	t.Run("abandoned create reaches no barrier", func(t *testing.T) {
		w, e := createRecorderWorld(t, kind)
		ready, _ := arm(w)
		token := createTokens[0]
		c := startCreate(t, w, e, token)
		c.abandon(t)
		e.finish()
		w.offline.api.awaitCreates()
		requireAbandonedAccounting(t, w, e, token)
		select {
		case <-ready:
			t.Error("an answer whose controller had left reached the reply barrier")
		default:
		}
	})
	t.Run("connected create is held", func(t *testing.T) {
		w, e := createRecorderWorld(t, kind)
		a := w.offline.api
		ready, release := arm(w)
		c := startCreate(t, w, e, createTokens[0])
		e.finish()
		select {
		case <-ready:
		case <-c.returned:
			t.Fatal("a connected controller's answer was not held at the reply barrier")
		case <-time.After(5 * time.Second):
			t.Fatal("a connected controller's answer never reached the reply barrier")
		}
		// Recording and inspection precede the hold, and the reply follows it.
		if listed := createListed(a); !reflect.DeepEqual(listed, []string{e.worker()}) {
			t.Errorf("held reply's worker is not listed: %v", listed)
		}
		if q := createEntry(t, a); !q.Replied.IsZero() || q.Observation["Id"] != e.worker() {
			t.Errorf("held reply was journalled as replied or uninspected: replied=%t", !q.Replied.IsZero())
		}
		if !c.pending() {
			t.Fatal("the reply escaped its barrier")
		}
		release()
		c.wait(t)
		if q := createEntry(t, a); c.response.Code != http.StatusCreated || q.Cancelled || q.Replied.IsZero() {
			t.Errorf("released reply = HTTP %d cancelled=%t replied=%t, want 201 delivered", c.response.Code, q.Cancelled, !q.Replied.IsZero())
		}
	})
}

func testCreateAllowance(t *testing.T, kind string, connected bool) {
	w, e := createRecorderWorld(t, kind)
	a := w.offline.api
	c := startCreate(t, w, e, createTokens[0])
	forwarded := time.Now()
	if connected {
		// No controller of the fixture waits this long; one that did reads the
		// transport failure when the allowance ends, never a held connection.
		select {
		case <-c.returned:
		case <-time.After(engineCreateOutcomeAllowance + time.Second):
			t.Errorf("a connected create outlived the %s allowance", engineCreateOutcomeAllowance)
			c.abandon(t)
		}
		if c.response.Code != http.StatusBadGateway || !strings.Contains(c.response.Body.String(), "offline fixture transport failed") {
			t.Errorf("connected controller read HTTP %d at the allowance, want 502 and the transport-failure body", c.response.Code)
		}
	} else {
		c.abandon(t)
	}
	a.awaitCreates()
	if elapsed := time.Since(forwarded); elapsed != engineCreateOutcomeAllowance {
		t.Errorf("the exchange ended %s after forwarding, want the %s allowance", elapsed, engineCreateOutcomeAllowance)
	}
	if listed := createListed(a); len(listed) != 0 {
		t.Errorf("an unanswered create listed a worker: %v", listed)
	}
	q := createEntry(t, a)
	if q.Status != http.StatusBadGateway || !strings.Contains(q.Diagnostic, "create outcome allowance") || !strings.Contains(q.Diagnostic, engineCreateOutcomeAllowance.String()) {
		t.Errorf("expired exchange journalled HTTP %d %q, want 502 and the fixed allowance diagnostic", q.Status, q.Diagnostic)
	}
	if q.Forwarded.IsZero() || !q.Answered.IsZero() || !q.Replied.IsZero() || q.Cancelled == connected {
		t.Errorf("expired exchange times: forwarded=%t answered=%t replied=%t cancelled=%t", !q.Forwarded.IsZero(), !q.Answered.IsZero(), !q.Replied.IsZero(), q.Cancelled)
	}
	problem := createProblem(a)
	switch {
	case connected:
		if problem != createTransportWords {
			t.Errorf("connected controller's expired exchange set %q, want %q", problem, createTransportWords)
		}
	case kind == "plain":
		// The engine-side failure is kept beside the abandonment.
		if !strings.Contains(problem, createAbandonedWords) || !strings.Contains(problem, createTransportWords) {
			t.Errorf("plain fixture problem = %q, want the abandonment kept beside the engine-side failure", problem)
		}
	case problem != "":
		t.Errorf("%s fixture set a recorder problem for an abandoned, expired create: %q", kind, problem)
	}
}

// The step wait returns only when every open exchange of the world and of its
// control, second and overlap worlds has ended, and holds no recorder lock.
func testCreateStepWait(t *testing.T) {
	(&world{}).awaitCreateOutcomes() // a scenario without a native recorder
	w, engine := createRecorderWorld(t, "plain")
	engines := []*createEngine{engine}
	recorders := []*offlineAPI{w.offline.api}
	child := func() *world {
		c, e := createRecorderWorld(t, "plain")
		engines, recorders = append(engines, e), append(recorders, c.offline.api)
		return c
	}
	w.anchor = &anchorWorld{control: child()}
	w.recovery = &recoveryNative{second: child()}
	w.lifecycle = &lifecycleNative{control: child()}
	w.overlap = &overlapFixture{worlds: map[string]*world{"ancestor": child(), "descendant": child()}}
	w.anchor.control.anchor = &anchorWorld{control: w} // shared references must stay finite
	worlds := []*world{w, w.anchor.control, w.recovery.second, w.lifecycle.control, w.overlap.worlds["ancestor"], w.overlap.worlds["descendant"]}
	for i, p := range worlds {
		startCreate(t, p, engines[i], createTokens[0]).abandon(t)
	}
	done := make(chan struct{})
	go func() { defer close(done); w.awaitCreateOutcomes() }()
	returned := func() bool {
		synctest.Wait()
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
	for i, e := range engines {
		if returned() {
			t.Fatalf("the step wait returned with %d exchanges still open", len(engines)-i)
		}
		for _, a := range recorders {
			if !a.mu.TryLock() {
				t.Fatal("the step wait holds a recorder lock while it waits")
			}
			a.mu.Unlock()
		}
		e.finish()
	}
	if !returned() {
		t.Fatal("the step wait outlived every exchange")
	}
	for i, a := range recorders {
		if listed := createListed(a); !reflect.DeepEqual(listed, []string{engines[i].worker()}) {
			t.Errorf("recorder %d returned from the step wait without its late answer: %v", i, listed)
		}
	}
}

func testCreateEngineFailure(t *testing.T) {
	t.Run("dropped connection", func(t *testing.T) {
		for _, kind := range []string{"plain", "recovery", "lifecycle"} {
			w, e := createRecorderWorld(t, kind)
			a := w.offline.api
			e.drop = true
			e.finish()
			c := startCreate(t, w, e, createTokens[0])
			c.wait(t)
			q := createEntry(t, a)
			if c.response.Code != http.StatusBadGateway || !strings.Contains(c.response.Body.String(), "offline fixture transport failed") ||
				q.Status != http.StatusBadGateway || !strings.HasPrefix(q.Diagnostic, "recorder transport: ") || q.Cancelled || !q.Replied.IsZero() {
				t.Errorf("%s: engine-side failure = HTTP %d, journal %d %q cancelled=%t", kind, c.response.Code, q.Status, q.Diagnostic, q.Cancelled)
			}
			want := createTransportWords
			if kind == "lifecycle" {
				want = ""
			}
			if problem := createProblem(a); problem != want {
				t.Errorf("%s: engine-side failure set %q, want %q", kind, problem, want)
			}
			if listed := createListed(a); len(listed) != 0 {
				t.Errorf("%s: failed exchange listed a worker: %v", kind, listed)
			}
		}
	})
	t.Run("refusal", func(t *testing.T) {
		w, e := createRecorderWorld(t, "plain")
		a := w.offline.api
		e.override = func(method, _ string) (int, any) {
			if method == "POST" {
				return http.StatusInternalServerError, map[string]any{"message": "synthetic engine refusal"}
			}
			return 0, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		response := httptest.NewRecorder()
		a.serve(response, createRequest(ctx, w, createTokens[0]))
		q := createEntry(t, a)
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "synthetic engine refusal") ||
			q.Status != http.StatusInternalServerError || q.Diagnostic != "engine response: synthetic engine refusal" || q.Cancelled || q.Replied.IsZero() {
			t.Errorf("engine refusal = HTTP %d, journal %d %q cancelled=%t replied=%t", response.Code, q.Status, q.Diagnostic, q.Cancelled, !q.Replied.IsZero())
		}
		if problem, listed := createProblem(a), createListed(a); problem != "" || len(listed) != 0 {
			t.Errorf("engine refusal set problem %q or listed %v", problem, listed)
		}
	})
	t.Run("reply without a full ID", func(t *testing.T) {
		w, e := createRecorderWorld(t, "plain")
		a := w.offline.api
		e.shortID = true
		e.finish()
		c := startCreate(t, w, e, createTokens[0])
		c.wait(t)
		if problem, listed := createProblem(a), createListed(a); c.response.Code != http.StatusCreated ||
			problem != "native create reply lacks a full container ID" || len(listed) != 0 {
			t.Errorf("short ID = HTTP %d problem %q listed %v", c.response.Code, problem, listed)
		}
	})
}

func testCreateOrdinary(t *testing.T) {
	for _, kind := range []string{"plain", "recovery", "lifecycle"} {
		w, e := createRecorderWorld(t, kind)
		a := w.offline.api
		e.finish()
		token := createTokens[0]
		c := startCreate(t, w, e, token)
		c.wait(t)
		id := e.worker()
		if c.response.Code != http.StatusCreated || !strings.Contains(c.response.Body.String(), id) {
			t.Errorf("%s: ordinary create = HTTP %d %q", kind, c.response.Code, c.response.Body.String())
		}
		if listed := createListed(a); !reflect.DeepEqual(listed, []string{id}) {
			t.Errorf("%s: ordinary create listed %v", kind, listed)
		}
		q := createEntry(t, a)
		if q.Status != http.StatusCreated || q.Cancelled || q.Replied.IsZero() || q.Observation["Id"] != id || q.Diagnostic != "" {
			t.Errorf("%s: ordinary create journalled %d cancelled=%t replied=%t", kind, q.Status, q.Cancelled, !q.Replied.IsZero())
		}
		if q.Forwarded.IsZero() || q.Answered.IsZero() || q.Forwarded.Before(q.At) || q.Answered.Before(q.Forwarded) || q.Replied.Before(q.Answered) {
			t.Errorf("%s: ordinary create lacks ordered forwarding, answer and reply times", kind)
		}
		a.mu.Lock()
		identity := a.identities[id]
		a.mu.Unlock()
		if identity["Name"] != "/itos-container-"+token {
			t.Errorf("%s: ordinary create identity = %v", kind, identity)
		}
		if problem := createProblem(a); problem != "" {
			t.Errorf("%s: ordinary create set a recorder problem: %q", kind, problem)
		}
		// The connected controller's own inspection needs no wait and no cleanup lookup.
		a.awaitCreates()
		inspected := httptest.NewRecorder()
		a.serve(inspected, httptest.NewRequest("GET", "http://engine/containers/"+id+"/json", nil))
		if inspected.Code != http.StatusOK {
			t.Errorf("%s: full-ID inspection after an ordinary create = HTTP %d", kind, inspected.Code)
		}
	}
}
