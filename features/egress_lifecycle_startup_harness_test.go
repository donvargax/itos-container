package features

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEgressLifecycleCurrentIntentIgnoresImageSidecar(t *testing.T) {
	w := &world{doctor: &doctorWorld{env: map[string]string{"XDG_STATE_HOME": t.TempDir()}}, approval: &approvalWorld{project: "project"}}
	root := w.egressStatePath("intents", w.approval.project)
	token := strings.Repeat("a", 64)
	for name, data := range map[string]string{
		token + ".json":        `{"anchor":{"stage":"owned"}}`,
		token + ".egress.json": `{"schema":1,"proxy_image_id":"image"}`,
	} {
		if err := writeManifestFixture(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	intent, err := w.egressLifeCurrentIntent()
	if err != nil || object(intent["anchor"])["stage"] != "owned" {
		t.Fatal("did not select the live intent beside its image sidecar", err)
	}
	if err := writeManifestFixture(filepath.Join(root, strings.Repeat("b", 64)+".json"), approvalJSON(intent), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.egressLifeCurrentIntent(); err == nil {
		t.Fatal("two live intents were accepted")
	}
}

func TestEgressLifecycleCreateBarrierHoldsOnlyAcceptedReply(t *testing.T) {
	for _, kind := range []string{"internal", "egress", "proxy"} {
		t.Run(kind, func(t *testing.T) {
			path, label := "/networks/create", kind+"-network"
			if kind == "proxy" {
				path, label = "/containers/create", "egress-proxy"
			}
			body := `{"Labels":{"io.itos-container.kind":"` + label + `"}}`
			reply := `{"Id":"` + strings.Repeat("a", 64) + `"}`
			b := newEgressLifeBarrier("create-" + kind)
			defer b.open()
			server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("reject") == "true" {
					out.WriteHeader(http.StatusConflict)
					return
				}
				out.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(out, reply)
			}))
			defer server.Close()
			a := &egressLifeRecorder{transport: http.DefaultTransport.(*http.Transport).Clone(), after: b.after}
			defer a.transport.CloseIdleConnections()
			a.before = func(e *egressLifeRequest) {
				a.mu.Lock()
				e.intent = map[string]any{"stage": "intent"}
				a.mu.Unlock()
			}
			request, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+path+"?reject=true", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			response, err := a.RoundTrip(request)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			select {
			case <-b.ready:
				t.Fatal("a rejected native create released the accepted-create barrier")
			default:
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			request, err = http.NewRequestWithContext(ctx, "POST", server.URL+path, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			type result struct {
				response *http.Response
				err      error
			}
			done := make(chan result, 1)
			go func() { r, e := a.RoundTrip(request); done <- result{r, e} }()
			select {
			case <-b.ready:
			case <-ctx.Done():
				t.Fatal("accepted create never reached its barrier")
			}
			select {
			case <-done:
				t.Fatal("accepted create reply escaped the barrier")
			default:
			}
			b.mu.Lock()
			observed, fault := b.request, b.err
			b.mu.Unlock()
			if fault != nil || string(observed.response) != reply || observed.intent["stage"] != "intent" {
				t.Fatal("barrier did not preserve original accepted reply and pre-create intent", fault)
			}
			b.open()
			got := <-done
			if got.err != nil {
				t.Fatal(got.err)
			}
			data, err := io.ReadAll(got.response.Body)
			_ = got.response.Body.Close()
			if err != nil || string(data) != reply || got.response.StatusCode != http.StatusCreated {
				t.Fatal("releasing barrier changed the real reply", err)
			}
			journal := a.snapshot()
			if len(journal) != 2 || string(journal[1].response) != reply || journal[1].finished.IsZero() {
				t.Fatal("journal lost accepted native ID")
			}
			journal[1].intent["stage"] = "tampered"
			if a.snapshot()[1].intent["stage"] != "intent" {
				t.Fatal("journal aliases the intent observation")
			}
		})
	}
}

func TestEgressLifecycleReadinessHoldAllowsFinalLogRead(t *testing.T) {
	b := newEgressLifeBarrier("ready-held")
	defer b.open()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", "http://engine/containers/original/logs?follow=1", nil)
	event := &egressLifeRequest{method: "GET", path: "/containers/original/logs", query: "follow=1"}
	stream := &egressUnreadBody{}
	done := make(chan error, 1)
	go func() { done <- b.after(request, event, &http.Response{StatusCode: 200, Body: stream}) }()
	select {
	case <-b.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("readiness call was not held")
	}
	final, _ := http.NewRequestWithContext(t.Context(), "GET", "http://engine/containers/original/logs?follow=0", nil)
	if err := b.after(final, &egressLifeRequest{method: "GET", path: event.path, query: "follow=0"}, &http.Response{StatusCode: 200, Body: stream}); err != nil {
		t.Fatal("final copy was held", err)
	}
	// Even a second matching request is passed through; this is a one-shot fault.
	if err := b.after(final, event, &http.Response{StatusCode: 200, Body: stream}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("held call did not honor cancellation", err)
	}
	if stream.reads != 0 {
		t.Fatal("readiness bytes were consumed by the fault")
	}
}

type egressUnreadBody struct{ reads int }

func (b *egressUnreadBody) Read([]byte) (int, error) { b.reads++; return 0, io.EOF }
func (*egressUnreadBody) Close() error               { return nil }

func TestEgressLifecycleBarrierKillFailureIsObserved(t *testing.T) {
	b := newEgressLifeBarrier("ready-killed")
	want := errors.New("independent kill refused")
	b.act = func(_ context.Context, id string) error {
		if id != "original" {
			t.Errorf("kill addressed %q", id)
		}
		return want
	}
	r, _ := http.NewRequestWithContext(t.Context(), "POST", "http://engine/containers/original/start", nil)
	err := b.after(r, &egressLifeRequest{method: "POST", path: "/containers/original/start"}, &http.Response{StatusCode: 204, Body: http.NoBody})
	if !errors.Is(err, want) || !errors.Is(b.err, want) {
		t.Fatal("harness failure became product evidence", err)
	}
	select {
	case <-b.ready:
	default:
		t.Fatal("failed independent kill was not observed")
	}
}

func TestEgressLifecycleRemovalOracleRequiresEveryCreatedIDAndAbsence(t *testing.T) {
	journal := []egressLifeRequest{
		{method: "POST", path: "/volumes/create", status: 201, response: []byte(`{"Name":"anchor"}`)},
		{method: "POST", path: "/networks/create", status: 201, response: []byte(`{"Id":"network"}`)},
		{method: "POST", path: "/containers/create", status: 201, response: []byte(`{"Id":"proxy"}`)},
		{method: "DELETE", path: "/containers/proxy", status: 204},
		{method: "GET", path: "/containers/proxy/json", status: 404},
		{method: "DELETE", path: "/networks/network", status: 204},
		{method: "GET", path: "/networks/network", status: 404},
		{method: "DELETE", path: "/volumes/anchor", status: 204},
		{method: "GET", path: "/volumes/anchor", status: 404},
	}
	if err := egressLifeReverseCreated(journal); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"missing absence", "missing removal", "foreign ID", "reversed", "failed removal", "failed create", "no ID"} {
		t.Run(change, func(t *testing.T) {
			copy := append([]egressLifeRequest(nil), journal...)
			switch change {
			case "missing absence":
				copy[4].status = 200
			case "missing removal":
				copy = append(copy[:5], copy[6:]...)
			case "foreign ID":
				copy[3].path = "/containers/foreign"
			case "reversed":
				copy[3], copy[5] = copy[5], copy[3]
			case "failed removal":
				copy[3].status = 500
			case "failed create":
				copy[2].status = 500
			case "no ID":
				copy[2].response = []byte(`{}`)
			}
			if err := egressLifeReverseCreated(copy); err == nil {
				t.Fatal("oracle accepted", change)
			}
		})
	}
}

func TestEgressLifecycleStoredStatusRequiresEvidenceAndCleanup(t *testing.T) {
	entry := map[string]any{"id": "session", "cleanup": "complete", "egress": map[string]any{"limitations": offlineStrings(egressLimitations), "refusal_log": map[string]any{"path": "/private/log", "state": "stored", "records": 0, "missing": []any{}}}}
	render := func() egressResult {
		return egressResult{runObservation: runObservation{result: doctorResult{stdout: string(approvalJSON(map[string]any{"schema": 1, "sessions": []any{entry}, "problems": []any{}}))}}}
	}
	if err := egressLifeStoredStatus(render(), "session", "/private/log", "stored", 0); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"missing evidence", "pending", "invented count", "retry", "no cleanup", "no limitation"} {
		t.Run(change, func(t *testing.T) {
			original := approvalJSON(entry)
			defer func() { entry = nil; _ = json.Unmarshal(original, &entry) }()
			if err := egressLifeStoredStatus(render(), "session", "/private/log", "stored", 0); err != nil {
				t.Fatal("positive control is not clean", err)
			}
			egress := object(entry["egress"])
			switch change {
			case "missing evidence":
				delete(egress, "refusal_log")
			case "pending":
				object(egress["refusal_log"])["state"] = "pending"
			case "invented count":
				object(egress["refusal_log"])["records"] = 1
			case "retry":
				entry["retry"] = []any{"itos", "container", "stop", "session"}
			case "no cleanup":
				entry["cleanup"] = "pending"
			case "no limitation":
				delete(egress, "limitations")
			}
			if err := egressLifeStoredStatus(render(), "session", "/private/log", "stored", 0); err == nil {
				t.Fatal("status oracle accepted", change)
			}
		})
	}
}

func TestEgressLifecycleStartupRejectsFallbackMutations(t *testing.T) {
	base := []egressLifeRequest{
		{method: "POST", path: "/networks/create", status: 201, body: []byte(`{"Labels":{"io.itos-container.kind":"internal-network"}}`), response: []byte(`{"Id":"internal"}`)},
		{method: "POST", path: "/networks/create", status: 201, body: []byte(`{"Labels":{"io.itos-container.kind":"egress-network"}}`), response: []byte(`{"Id":"egress"}`)},
		{method: "POST", path: "/containers/create", status: 201, body: []byte(`{"Labels":{"io.itos-container.kind":"egress-proxy"}}`), response: []byte(`{"Id":"proxy"}`)},
		{method: "POST", path: "/networks/egress/connect", status: 200, body: []byte(`{"Container":"proxy"}`)},
		{method: "POST", path: "/containers/proxy/start", status: 204},
		{method: "POST", path: "/containers/proxy/kill", status: 204},
		{method: "POST", path: "/containers/proxy/wait", status: 200},
	}
	if err := egressLifeStartupRoutes(base); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []egressLifeRequest{
		{method: "POST", path: "/networks/foreign/connect", body: []byte(`{"Container":"proxy"}`)},
		{method: "POST", path: "/networks/internal/connect", body: []byte(`{"Container":"worker"}`)},
		{method: "POST", path: "/networks/egress/connect", body: []byte(`{"Container":"proxy"}`)},
		{method: "POST", path: "/containers/proxy/restart"},
		{method: "POST", path: "/containers/other/start"},
		{method: "POST", path: "/containers/proxy/exec"},
		{method: "PUT", path: "/containers/proxy/archive"},
	} {
		if err := egressLifeStartupRoutes(append(append([]egressLifeRequest{}, base...), extra)); err == nil {
			t.Errorf("accepted unexpected %s %s", extra.method, extra.path)
		}
	}
}

func TestEgressLifecycleReadinessTimingUsesHarnessBudgets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		elapsed time.Duration
		want    bool
	}{
		{"call expires", engineCallBudget + 10*time.Millisecond, true},
		{"premature failure", engineCallBudget / 2, false},
		{"teardown exceeds budget", engineCallBudget + 33*time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := time.Now()
			barrier := newEgressLifeBarrier("ready-held")
			barrier.request.at = at
			run := &egressNativeRun{exit: 75}
			_, _ = run.stderr.Write([]byte("run.call-timeout"))
			w := &world{egress: &egressWorld{lifeStartup: &egressLifeStartup{barrier: barrier, run: run, reached: true, started: at, ended: at.Add(tc.elapsed)}}}
			if err := w.egressLifeStartupResult("run.call-timeout", 75); (err == nil) != tc.want {
				t.Fatalf("timing oracle = %v, want accepted %v", err, tc.want)
			}
		})
	}
}
