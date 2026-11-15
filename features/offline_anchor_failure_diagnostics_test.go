package features

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAnchorOriginalFailureDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, diagnostic, reason string
		cancelled, reached       bool
		exit, frames             int
	}{
		{"cancelled start", "recorder transport: context canceled private details", "cancelled", true, true, 75, 0},
		{"deadline before start", "recorder transport: context deadline exceeded private details", "deadline", false, false, 75, 0},
		{"transport loss after probe", "recorder transport: private details", "unclassified", false, true, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := errors.New("offline native recorder transport failed")
			start := time.Unix(100, 0)
			requests := []offlineRequest{
				{Method: "GET", Path: "/info", Status: 200, At: start, Replied: start.Add(50 * time.Millisecond)},
				{Method: "POST", Path: "/containers/private-id/start", Status: 502, Cancelled: tc.cancelled, Diagnostic: tc.diagnostic,
					Query: "private-query", Body: map[string]any{"private": "create body"}, Observation: map[string]any{"private": "inspection"}},
			}
			// No engine or process is available: reporting must use only the
			// retained observations, without retrying or collecting new evidence.
			w := &world{
				anchor: &anchorWorld{err: failure, held: tc.reached, reached: tc.reached, reports: make([]anchorReport, tc.frames)},
				offline: &offlineRuntime{exit: tc.exit, stdout: []byte("private workload output"), stderr: []byte("private controller output"),
					api: &offlineAPI{requests: requests}},
			}
			err := w.anchorOriginal()
			if !errors.Is(err, failure) {
				t.Fatalf("diagnosis lost the original observation failure: %v", err)
			}
			wants := []string{"native anchor observations unavailable", "GET /info HTTP=200", "reply-after=50ms",
				"POST /containers/<resource>/start HTTP=502", "reason=" + tc.reason}
			if tc.reached {
				wants = append(wants, "submitted-start reached=true held=true")
			} else {
				wants = append(wants, "submitted-start reached=false held=false")
			}
			if tc.cancelled {
				wants = append(wants, "cancelled=true")
			} else {
				wants = append(wants, "HTTP=502 cancelled=false")
			}
			if tc.frames == 0 {
				wants = append(wants, "controller exit=75", "probe-frames=0")
			} else {
				wants = append(wants, "controller exit=0", "probe-frames=2")
			}
			for _, want := range wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("failure omitted %q: %v", want, err)
				}
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("diagnostics exposed untrusted output or request contents", err)
			}
			if len(w.offline.api.requests) != 2 || w.anchor.err != failure {
				t.Fatal("diagnosis changed the recorded failure")
			}
		})
	}
}
