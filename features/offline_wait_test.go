package features

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A local HTTP peer models Docker's immediate wait acknowledgement followed by
// a body withheld until workload exit. No engine or product result is faked.
func TestOfflineRecorderStreamsWaitUntilExitOrCancellation(t *testing.T) {
	for _, cancelWait := range []bool{false, true} {
		name := "exit"
		if cancelWait {
			name = "cancellation"
		}
		t.Run(name, func(t *testing.T) {
			id := strings.Repeat("a", 64)
			finish := make(chan struct{})
			cancelled := make(chan struct{})
			var w *world
			w = offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/json") {
					_ = json.NewEncoder(out).Encode(offlineStartupInspection(w, id))
					return
				}
				if r.Method != "POST" || r.URL.Path != "/containers/"+id+"/wait" {
					t.Error("unexpected fixture request", r.URL.Path)
					http.Error(out, "unexpected request", http.StatusBadRequest)
					return
				}
				out.Header().Set("Content-Type", "application/json")
				out.WriteHeader(http.StatusOK)
				out.(http.Flusher).Flush()
				select {
				case <-finish:
					_, _ = io.WriteString(out, `{"StatusCode":7}`)
				case <-r.Context().Done():
					close(cancelled)
				}
			}))
			w.offline.api.created = []string{id}
			transport := doctorTransport(strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://"))
			defer transport.CloseIdleConnections()
			// Only this scenario budget bounds the forwarded wait stream. The
			// recorder's ordinary inspection client retains its short timeout.
			client := &http.Client{Transport: transport}
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, "POST", "http://engine/containers/"+id+"/wait?condition=not-running", nil)
			if err != nil {
				t.Fatal(err)
			}
			type reply struct {
				response *http.Response
				err      error
			}
			headers := make(chan reply, 1)
			go func() {
				response, err := client.Do(request)
				headers <- reply{response, err}
			}()
			var response *http.Response
			select {
			case got := <-headers:
				if got.err != nil {
					t.Fatal("wait acknowledgement failed", got.err)
				}
				response = got.response
			case <-time.After(time.Second):
				cancel()
				got := <-headers
				if got.response != nil {
					_ = got.response.Body.Close()
				}
				t.Fatal("recorder buffered the wait acknowledgement behind workload exit")
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != http.StatusOK {
				t.Fatal("wait acknowledgement status", response.StatusCode)
			}
			if cancelWait {
				cancel()
				select {
				case <-cancelled:
				case <-time.After(time.Second):
					t.Fatal("scenario cancellation did not reach the pending wait")
				}
				return
			}
			body := make(chan string, 1)
			go func() {
				data, err := io.ReadAll(response.Body)
				if err != nil {
					body <- err.Error()
					return
				}
				body <- string(data)
			}()
			// Exceed both the read-only client's five seconds and the product's
			// ten-second acknowledgement budget without ending the workload.
			select {
			case got := <-body:
				t.Fatal("recorder ended wait before workload exit", got)
			case <-time.After(11 * time.Second):
			}
			close(finish)
			select {
			case got := <-body:
				if got != `{"StatusCode":7}` {
					t.Fatal("wait body changed", got)
				}
			case <-ctx.Done():
				t.Fatal("workload exit never reached the client")
			}
			a := w.offline.api
			a.mu.Lock()
			defer a.mu.Unlock()
			if len(a.requests) != 1 || a.requests[0].Status != http.StatusOK ||
				a.requests[0].Observation["StatusCode"] != float64(7) || a.requests[0].Diagnostic != "" {
				t.Fatal("streaming lost the wait journal", a.requests)
			}
		})
	}
}
