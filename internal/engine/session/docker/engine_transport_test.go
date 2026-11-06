package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestRemovalRechecksSocketAfterConnectionWarmup(t *testing.T) {
	for _, capability := range []string{"unstarted", "foreground"} {
		t.Run(capability, func(t *testing.T) {
			id := strings.Repeat("c", 64)
			var removals atomic.Int32
			engine, b, pin := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1.56/info":
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]string{"ID": b.EngineID})
				case r.Method == http.MethodDelete && r.URL.Path == "/v1.56/containers/"+id:
					removals.Add(1)
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected engine request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var conn sp.Connection
			var failure sp.Outcome
			if capability == "foreground" {
				conn, failure = engine.OpenRun(ctx, b)
			} else {
				conn, failure = engine.Open(ctx, b)
			}
			if failure != "" {
				t.Fatal(failure)
			}
			defer conn.Close()
			remove := conn.Remove
			if run, ok := conn.(sp.RunConnection); ok {
				remove = run.RemoveRun
			}
			// Leave a successful HTTP exchange available for reuse if the
			// transport incorrectly pools connections.
			if failure = conn.Check(ctx); failure != "" {
				t.Fatal("healthy socket control failed", failure)
			}
			// GetConn runs after Remove's initial identity check, even when
			// an idle connection is available. Reusing it would bypass the
			// dial-time pin check and send a mutation after substitution.
			var acquiring atomic.Bool
			changed := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
				GetConn: func(string) {
					pin.Current = false
					acquiring.Store(true)
				},
			})
			remove(changed, id)
			if !acquiring.Load() {
				t.Fatal("removal never reached transport connection acquisition")
			}
			if got := removals.Load(); got != 0 {
				t.Fatalf("sent %d removals after the pinned socket changed during connection acquisition", got)
			}
			pin.Current = true
			remove(ctx, id)
			if got := removals.Load(); got != 1 {
				t.Fatalf("healthy socket removal control sent %d requests, want one", got)
			}
		})
	}
}
