package features

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
)

// A create becomes independent of its controller only after the recorder has
// admitted it for forwarding. Pre-forward inspection must not revive a request
// whose controller has already left.
func TestOfflineRecorderRefusesCreateCancelledBeforeForwarding(t *testing.T) {
	for _, kind := range []string{"plain", "recovery", "lifecycle"} {
		for _, when := range []string{"before serve", "during anchor inspection"} {
			t.Run(kind+"/"+when, func(t *testing.T) {
				w, e := createRecorderWorld(t, kind)
				a := w.offline.api
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				request := createRequest(ctx, w, createTokens[0])
				if when == "before serve" {
					cancel()
				} else {
					v := e.anchor(w, createTokens[0])
					var body map[string]any
					if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					mounts := object(body["HostConfig"])["Mounts"].([]any)
					mounts[0] = map[string]any{"Type": "volume", "Source": v.name, "Target": "/workspace", "VolumeOptions": map[string]any{"NoCopy": true}}
					request.Body = io.NopCloser(strings.NewReader(string(approvalJSON(body))))
					e.override = func(method, uri string) (int, any) {
						if method == "GET" && uri == "/volumes/"+v.name {
							cancel() // allowed() has not yet returned to serve.
						}
						return 0, nil
					}
				}
				e.finish()
				response := httptest.NewRecorder()
				a.serve(response, request)
				a.awaitCreates()
				requireCreateNotForwarded(t, a, e, response)
				q := createEntry(t, a)
				if !q.Cancelled || !strings.Contains(q.Diagnostic, "context canceled") {
					t.Errorf("pre-forward cancellation lacks its journal cause: cancelled=%t diagnostic=%q", q.Cancelled, q.Diagnostic)
				}
			})
		}
	}
}

func TestOfflineRecorderRefusesCreateAfterCleanupStarts(t *testing.T) {
	t.Run("while cleanup waits for a forwarded create", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w, e := createRecorderWorld(t, "recovery")
			a := w.offline.api
			// If the second create slips through, answer it immediately rather than
			// making the regression depend on the create-outcome allowance.
			e.override = func(method, uri string) (int, any) {
				if method == "POST" && strings.Contains(uri, createTokens[1]) {
					return http.StatusInternalServerError, map[string]any{"message": "unexpected second create"}
				}
				return 0, nil
			}
			startCreate(t, w, e, createTokens[0]).abandon(t)
			done := make(chan error, 1)
			go func() { done <- a.cleanup() }()
			synctest.Wait()
			select {
			case err := <-done:
				t.Fatalf("cleanup returned before its forwarded create ended: %v", err)
			default:
			}
			response := httptest.NewRecorder()
			a.serve(response, createRequest(t.Context(), w, createTokens[1]))
			if response.Code != http.StatusBadGateway {
				t.Errorf("create during cleanup = HTTP %d, want 502", response.Code)
			}
			for _, q := range e.journal() {
				if strings.Contains(q, createTokens[1]) {
					t.Errorf("cleanup admitted another engine request: %s", q)
				}
			}
			a.mu.Lock()
			q := a.requests[len(a.requests)-1]
			a.mu.Unlock()
			if q.Status != http.StatusBadGateway || !q.Forwarded.IsZero() || !q.Answered.IsZero() || !q.Replied.IsZero() || q.Diagnostic == "" {
				t.Errorf("create during cleanup was not journalled as an unforwarded failure: %+v", q)
			}
			e.finish()
			if err := <-done; err != nil {
				t.Errorf("cleanup of the already forwarded create: %v", err)
			}
			if e.worker() != "" {
				t.Error("cleanup left the already forwarded worker in the engine")
			}
		})
	})
	t.Run("after cleanup finished", func(t *testing.T) {
		w, e := createRecorderWorld(t, "plain")
		a := w.offline.api
		if err := a.cleanup(); err != nil {
			t.Fatal(err)
		}
		e.finish()
		response := httptest.NewRecorder()
		a.serve(response, createRequest(t.Context(), w, createTokens[0]))
		a.awaitCreates()
		requireCreateNotForwarded(t, a, e, response)
	})
}

func requireCreateNotForwarded(t *testing.T, a *offlineAPI, e *createEngine, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "offline fixture transport failed") {
		t.Errorf("unforwarded create = HTTP %d %q, want 502 and the transport-failure body", response.Code, response.Body.String())
	}
	for _, q := range e.journal() {
		if strings.HasPrefix(q, "POST /containers/create") {
			t.Errorf("engine received a create that should not have been forwarded: %s", q)
		}
	}
	if e.worker() != "" || len(createListed(a)) != 0 {
		t.Error("unforwarded create left a worker in the engine or recorder")
	}
	q := createEntry(t, a)
	if q.Status != http.StatusBadGateway || !q.Forwarded.IsZero() || !q.Answered.IsZero() || !q.Replied.IsZero() || q.Diagnostic == "" {
		t.Errorf("unforwarded create lacks its failure without exchange times: %+v", q)
	}
}
