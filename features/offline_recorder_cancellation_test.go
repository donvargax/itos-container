package features

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Wait is a subscription. A controller may cancel it during owned cleanup or
// stdin-close reconciliation; that is incomplete wait evidence, not a failed
// observation of all other native requests. Mutation/inspection failures remain fatal.
func TestOfflineRecorderCancellationEvidence(t *testing.T) {
	for _, mode := range []string{"cancelled wait", "broken wait transport", "cancelled inspection"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
				if mode != "broken wait transport" {
					cancel()
					// Exercise cancellation before any response headers. Returning
					// immediately races the transport against net/http's implicit
					// 200, which instead exercises a canceled response body.
					<-request.Context().Done()
					return
				}
				conn, _, err := out.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
			}))
			a := w.offline.api
			id := strings.Repeat("a", 64)
			a.created = []string{id}
			method, path := "POST", "/containers/"+id+"/wait?condition=not-running"
			if mode == "cancelled inspection" {
				method, path = "GET", "/info"
			}
			request := httptest.NewRequest(method, "http://engine"+path, nil).WithContext(ctx)
			a.serve(httptest.NewRecorder(), request)
			a.mu.Lock()
			defer a.mu.Unlock()
			if len(a.requests) != 1 {
				t.Fatalf("journal lost request: %#v", a.requests)
			}
			entry := a.requests[0]
			if entry.Status != http.StatusBadGateway || entry.Diagnostic == "" || entry.Cancelled != (mode != "broken wait transport") || !entry.Replied.IsZero() || entry.Observation != nil {
				t.Fatalf("failed request became completed evidence: %#v", entry)
			}
			if mode == "cancelled wait" {
				if a.problem != nil {
					t.Fatalf("cancelled subscription poisoned native observations: %v", a.problem)
				}
			} else if a.problem == nil {
				t.Fatal("required native observation failure was ignored")
			} else if mode == "cancelled inspection" && !errors.Is(a.problem, context.Canceled) {
				t.Fatalf("transport cause was lost: %v", a.problem)
			}
		})
	}
}
