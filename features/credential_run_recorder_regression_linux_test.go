package features

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	credentialRecorderHold   = 100 * time.Millisecond
	credentialRecorderCreate = "/v1.47/containers/create"
)

var credentialRecorderWorker = strings.Repeat("a", 64)

type credentialRecorderCall struct {
	method, path string
	at           time.Time
}

type credentialRecorderReply struct {
	status int
	body   string
	err    error
}

type credentialRecorderFixture struct {
	*credentialRunJournal
	client *http.Client
	mu     sync.Mutex
	calls  []credentialRecorderCall
}

func newCredentialRecorderFixture(t *testing.T) *credentialRecorderFixture {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "crr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	upstream := filepath.Join(root, "upstream.sock")
	listener, err := net.Listen("unix", upstream)
	if err != nil {
		t.Fatal(err)
	}
	f := &credentialRecorderFixture{}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(f.respond)}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	socket := filepath.Join(root, "recorder.sock")
	f.credentialRunJournal, err = newCredentialRunRecorder(socket, upstream, credentialRecorderHold)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.close() })
	transport := doctorTransport(socket)
	t.Cleanup(transport.CloseIdleConnections)
	f.client = &http.Client{Transport: transport, Timeout: 3 * time.Second}
	return f
}

func (f *credentialRecorderFixture) respond(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, credentialRecorderCall{method: r.Method, path: r.URL.Path, at: time.Now()})
	f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && credentialRunAPIPath(r.URL.Path) == "/containers/create":
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"Id":"%s"}`, credentialRecorderWorker)
	case r.Method == http.MethodGet:
		_, _ = io.WriteString(w, "{}")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (f *credentialRecorderFixture) matching(method, path string) []credentialRecorderCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []credentialRecorderCall
	for _, call := range f.calls {
		if call.method == method && call.path == path {
			result = append(result, call)
		}
	}
	return result
}

func (f *credentialRecorderFixture) request(ctx context.Context, method, path string) credentialRecorderReply {
	request, err := http.NewRequestWithContext(ctx, method, "http://daemon"+path, nil)
	if err != nil {
		return credentialRecorderReply{err: err}
	}
	reply, err := f.client.Do(request)
	if err != nil {
		return credentialRecorderReply{err: err}
	}
	defer func() { _ = reply.Body.Close() }()
	body, err := io.ReadAll(reply.Body)
	return credentialRecorderReply{status: reply.StatusCode, body: string(body), err: err}
}

func (f *credentialRecorderFixture) expect(t *testing.T, method, path string, status int, body string) {
	t.Helper()
	reply := f.request(t.Context(), method, path)
	if reply.err != nil || reply.status != status || reply.body != body {
		t.Fatalf("%s %s returned status %d body %q error %v", method, path, reply.status, reply.body, reply.err)
	}
}

func (f *credentialRecorderFixture) create(t *testing.T, path string) {
	t.Helper()
	f.expect(t, http.MethodPost, path, http.StatusCreated, `{"Id":"`+credentialRecorderWorker+`"}`)
}

func credentialRecorderReceive[T any](t *testing.T, notification <-chan T) T {
	t.Helper()
	select {
	case value := <-notification:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("recorder notification deadline expired")
		var zero T
		return zero
	}
}

func credentialRecorderAwait(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-deadline.C:
			t.Fatal("recorder observation deadline expired")
		case <-tick.C:
		}
	}
}

func TestCredentialRunRecoveryRecorder(t *testing.T) {
	path := "/v1.47/containers/" + credentialRecorderWorker
	t.Run("unarmed_passthrough", func(t *testing.T) {
		f := newCredentialRecorderFixture(t)
		f.create(t, credentialRecorderCreate)
		f.expect(t, http.MethodPost, path+"/start", http.StatusNoContent, "")
		f.expect(t, http.MethodPost, path+"/kill", http.StatusNoContent, "")
		f.expect(t, http.MethodPost, path+"/stop", http.StatusNoContent, "")
		f.expect(t, http.MethodGet, path+"/json", http.StatusOK, "{}")
		f.expect(t, http.MethodDelete, path, http.StatusNoContent, "")
		if f.faultLog() != (credentialRunFaultLog{}) {
			t.Fatal("an unarmed recorder logged a fault")
		}
		entries := f.snapshot()
		statuses := []int{201, 204, 204, 204, 200, 204}
		if len(entries) != len(statuses) {
			t.Fatalf("journal has %d entries, expected %d", len(entries), len(statuses))
		}
		for i, entry := range entries {
			calls := f.matching(entry.Method, entry.Path)
			if entry.At.IsZero() || entry.Fault != "" || entry.Status != statuses[i] || len(calls) != 1 || calls[0].at.Before(entry.At) {
				t.Fatalf("pass-through call %d did not reach the fake and journal unchanged", i)
			}
		}
	})
	t.Run("lost_start", func(t *testing.T) {
		f := newCredentialRecorderFixture(t)
		f.arm(credentialRunFaults{LoseStart: true})
		f.create(t, credentialRecorderCreate)
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		reply := f.request(ctx, http.MethodPost, path+"/start")
		if reply.err == nil || reply.status != 0 || ctx.Err() != context.DeadlineExceeded {
			t.Fatal("start client received a reply or failed before its own deadline")
		}
		if worker := credentialRecorderReceive(t, f.accepted()); worker != credentialRecorderWorker {
			t.Fatal("start acceptance reported a different worker")
		}
		credentialRecorderAwait(t, func() bool { return f.faultLog().StartReplyWithheld })
		if len(f.matching(http.MethodPost, path+"/start")) != 1 {
			t.Fatal("first start was not sent upstream exactly once")
		}
		f.expect(t, http.MethodPost, path+"/start", http.StatusConflict, credentialRepeatReply)
		if len(f.matching(http.MethodPost, path+"/start")) != 1 || f.faultLog().Starts != 2 {
			t.Fatal("repeated start reached upstream or was not counted")
		}
		journal := f.snapshot()
		if len(journal) != 3 || journal[1].Status != 204 || journal[1].Fault == "" ||
			journal[2].Status != 409 || string(journal[2].ResponseBody) != credentialRepeatReply {
			t.Fatal("accepted and refused starts were not both preserved in the journal")
		}
	})
	t.Run("ending_released", func(t *testing.T) {
		f := newCredentialRecorderFixture(t)
		f.arm(credentialRunFaults{LoseStart: true, StagedAbsent: func() (bool, error) { return true, nil }})
		f.create(t, credentialRecorderCreate)
		done := make(chan credentialRecorderReply, 1)
		go func() { done <- f.request(t.Context(), http.MethodPost, path+"/kill") }()
		credentialRecorderReceive(t, f.faults.endingArrived)
		ending := f.faultLog().Ending
		timer := time.NewTimer(time.Until(ending.Arrived.Add(30 * time.Millisecond)))
		defer timer.Stop()
		<-timer.C
		if len(f.matching(http.MethodPost, path+"/kill")) != 0 {
			t.Fatal("first kill reached upstream before release")
		}
		released := time.Now()
		f.release()
		f.release()
		if reply := credentialRecorderReceive(t, done); reply.status != 204 || reply.err != nil {
			t.Fatal("released kill did not succeed")
		}
		ending = f.faultLog().Ending
		calls := f.matching(http.MethodPost, path+"/kill")
		if !ending.Released || ending.Expired || !ending.StagedAbsentAtArrival || len(calls) != 1 || calls[0].at.Before(released) {
			t.Fatal("recorder did not observe and release the first kill")
		}
		f.expect(t, http.MethodPost, path+"/kill", 204, "")
		if *f.faultLog().Ending != *ending || len(f.matching(http.MethodPost, path+"/kill")) != 2 {
			t.Fatal("second kill was held or not forwarded")
		}
	})
	t.Run("ending_expired", func(t *testing.T) {
		f := newCredentialRecorderFixture(t)
		f.arm(credentialRunFaults{LoseStart: true, StagedAbsent: func() (bool, error) { return false, nil }})
		f.create(t, credentialRecorderCreate)
		f.expect(t, http.MethodPost, path+"/stop", 204, "")
		ending := f.faultLog().Ending
		calls := f.matching(http.MethodPost, path+"/stop")
		if ending == nil || !ending.Expired || ending.Released || ending.StagedAbsentAtArrival ||
			len(calls) != 1 || calls[0].at.Sub(ending.Arrived) < credentialRecorderHold {
			t.Fatal("first stop reached the fake early or did not record expiry")
		}
	})
	t.Run("removal_refused_until_clear", func(t *testing.T) {
		f := newCredentialRecorderFixture(t)
		f.arm(credentialRunFaults{LoseStart: true, FailRemoval: true})
		f.create(t, credentialRecorderCreate)
		f.expect(t, http.MethodDelete, path+"?force=1", 503, credentialRemoveReply)
		ending := f.faultLog().Ending
		if len(f.matching(http.MethodDelete, path)) != 0 || ending == nil || !ending.Expired {
			t.Fatal("first delete bypassed the hold or reached the fake")
		}
		f.clear()
		f.clear()
		f.expect(t, http.MethodDelete, path, 204, "")
		if f.faultLog().RefusedRemovals != 1 || len(f.matching(http.MethodDelete, path)) != 1 {
			t.Fatal("recorder did not resume deletion after clear")
		}
	})
	t.Run("barrier_create", func(t *testing.T) {
		testCredentialRecorderBarrier(t, credentialCreateBarrier, credentialRecorderCreate, 201)
	})
	t.Run("barrier_start", func(t *testing.T) {
		testCredentialRecorderBarrier(t, credentialStartBarrier, path+"/start", 204)
	})
	t.Run("prefix_and_identity", func(t *testing.T) {
		f := newCredentialRecorderFixture(t)
		f.arm(credentialRunFaults{LoseStart: true, FailRemoval: true})
		f.create(t, "/containers/create")
		other := "/containers/" + strings.Repeat("b", 64)
		f.expect(t, http.MethodPost, other+"/start", 204, "")
		f.expect(t, http.MethodPost, other+"/kill", 204, "")
		f.expect(t, http.MethodDelete, other, 204, "")
		log := f.faultLog()
		if log.Worker != credentialRecorderWorker || log.Creates != 1 || log.Starts != 0 || log.RefusedRemovals != 0 || log.Ending != nil {
			t.Fatal("recorder applied a fault to a different worker or failed to match an unversioned path")
		}
		f.release()
		f.expect(t, http.MethodDelete, "/containers/"+credentialRecorderWorker, 503, credentialRemoveReply)
	})
}

func testCredentialRecorderBarrier(t *testing.T, barrier, path string, status int) {
	t.Helper()
	f := newCredentialRecorderFixture(t)
	f.arm(credentialRunFaults{Barrier: barrier})
	if barrier == credentialStartBarrier {
		f.create(t, credentialRecorderCreate)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan credentialRecorderReply, 1)
	go func() { done <- f.request(ctx, http.MethodPost, path) }()
	if worker := credentialRecorderReceive(t, f.held()); worker != credentialRecorderWorker {
		t.Fatal("held barrier reported a different worker")
	}
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-done:
		t.Fatal("barrier returned a reply before cancellation")
	case <-timer.C:
	}
	cancel()
	if reply := credentialRecorderReceive(t, done); reply.status != 0 || reply.err == nil {
		t.Fatal("barrier client received a reply")
	}
	credentialRecorderReceive(t, f.faults.barrierEnded)
	log := f.faultLog()
	if log.Barrier != barrier || log.BarrierAt.IsZero() || !log.BarrierEnded.After(log.BarrierAt) ||
		len(f.matching(http.MethodPost, path)) != 1 {
		t.Fatal("recorder did not observe one held acceptance and its cancellation")
	}
	entries := f.snapshot()
	last := entries[len(entries)-1]
	if last.Status != status || last.Fault == "" || last.Truncated ||
		status == 201 && string(last.ResponseBody) != `{"Id":"`+credentialRecorderWorker+`"}` {
		t.Fatal("held daemon reply was not preserved in the journal")
	}
	if err := f.scan(func(string, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
