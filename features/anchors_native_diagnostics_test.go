package features

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Capture existing observations before scenario cleanup destroys the world.
// Diagnostics never retry a product command or query the engine. Only known
// enum values and request metadata are emitted, never workload streams,
// settings, record contents, query strings or raw engine error messages.
func (w *world) anchorRecoveryDiagnostics() string {
	var out strings.Builder
	if r := w.recovery; r != nil {
		anchorProcessDiagnostic(&out, "status", r.status)
		for i, p := range r.stops {
			anchorProcessDiagnostic(&out, fmt.Sprintf("stop-%d", i+1), p)
		}
		anchorProcessDiagnostic(&out, "status-after", r.after)
	}
	if w.offline != nil && w.offline.api != nil {
		a := w.offline.api
		a.mu.Lock()
		defer a.mu.Unlock()
		// Keep the first recovery requests as well as the tail: the status
		// which failed can precede several complete stop attempts.
		start := 0
		if w.anchors != nil {
			start = min(w.anchors.recoveryIndex, len(a.requests))
		}
		fmt.Fprintf(&out, "product requests=%d showing-from=%d\n", len(a.requests), start)
		for i, q := range a.requests[start:] {
			if i >= 32 && i < len(a.requests)-start-32 {
				continue
			}
			method := anchorDiagnosticValue(q.Method, "GET", "HEAD", "POST", "DELETE")
			fmt.Fprintf(&out, "%s %s HTTP=%d cancelled=%t replied=%t reason=%s\n", method, anchorDiagnosticRoute(q.Path), q.Status, q.Cancelled, !q.Replied.IsZero(), anchorDiagnosticReason(q.Diagnostic))
			if !q.At.IsZero() && !q.Replied.IsZero() {
				fmt.Fprintf(&out, "reply-after=%s\n", q.Replied.Sub(q.At))
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func anchorProcessDiagnostic(out *strings.Builder, label string, p *recoveryProcess) {
	if p == nil {
		return
	}
	select {
	case <-p.done:
	default:
		fmt.Fprintf(out, "%s pending\n", label)
		return
	}
	fmt.Fprintf(out, "%s exit=%d elapsed=%s\n", label, p.exit, p.finished.Sub(p.started))
	var report struct {
		Schema   int
		OK       bool
		Problems []struct{ Rule string }
		Sessions []struct{ State, Resource, Anchor, Controller, Cleanup string }
	}
	data := p.out.bytes()
	if len(data) > 1<<20 || json.Unmarshal(data, &report) != nil {
		fmt.Fprintln(out, "report=unavailable")
		return
	}
	fmt.Fprintf(out, "schema=%d ok=%t problems=%d sessions=%d\n", report.Schema, report.OK, len(report.Problems), len(report.Sessions))
	for _, problem := range report.Problems[:min(4, len(report.Problems))] {
		fmt.Fprintln(out, anchorDiagnosticValue(problem.Rule, "recovery.internal", "recovery.usage", "recovery.not-owned", "recovery.unsafe-state", "recovery.invalid-state", "recovery.environment", "recovery.temporary"))
	}
	for _, s := range report.Sessions[:min(4, len(report.Sessions))] {
		fmt.Fprintf(out, "state=%s resource=%s anchor=%s controller=%s cleanup=%s\n",
			anchorDiagnosticValue(s.State, "preparing", "ready", "running", "stopping", "stopped", "failed"),
			anchorDiagnosticValue(s.Resource, "unknown", "running", "created", "exited", "absent"),
			anchorDiagnosticValue(s.Anchor, "unknown", "present", "absent"),
			anchorDiagnosticValue(s.Controller, "unknown", "present", "absent"),
			anchorDiagnosticValue(s.Cleanup, "unknown", "pending", "complete"))
	}
}

func anchorDiagnosticValue(value string, allowed ...string) string {
	for _, known := range allowed {
		if value == known {
			return known
		}
	}
	return "unrecognized"
}

func anchorDiagnosticRoute(path string) string {
	if strings.HasPrefix(path, "/volumes/") {
		return "/volumes/<resource>"
	}
	if target, ok := strings.CutPrefix(path, "/containers/"); ok {
		_, operation, _ := strings.Cut(target, "/")
		return "/containers/<resource>/" + anchorDiagnosticValue(operation, "", "json", "start", "wait", "kill", "stop", "attach")
	}
	return anchorDiagnosticValue(path, "/info", "/_ping", "/version")
}

func anchorDiagnosticReason(message string) string {
	switch {
	case message == "":
		return "none"
	case strings.Contains(message, "is in use"):
		return "in-use"
	case strings.Contains(message, "device or resource busy"):
		return "busy"
	case strings.Contains(message, "context deadline exceeded"):
		return "deadline"
	case strings.Contains(message, "context canceled"):
		return "cancelled"
	case message == "recorder refused unsafe request":
		return "recorder-refusal"
	case message == "recorder injected removal fault":
		return "injected-removal-fault"
	default:
		return "unclassified"
	}
}

func TestAnchorRecoveryExitDiagnostics(t *testing.T) {
	start := time.Unix(100, 0)
	process := func(report string, code int) *recoveryProcess {
		p := &recoveryProcess{done: make(chan struct{}), exit: code, started: start, finished: start.Add(time.Second)}
		_, _ = p.out.Write([]byte(report))
		_, _ = p.errout.Write([]byte("private workload output"))
		close(p.done)
		return p
	}
	status := process(`{"schema":1,"ok":false,"problems":[{"rule":"recovery.temporary","message":"private message","fix":"private fix"}],"sessions":[{"state":"failed","resource":"running","anchor":"unknown","controller":"absent","cleanup":"unknown","workload_exit":null}]}`, 75)
	stop := process(`{"schema":1,"ok":false,"problems":[{"rule":"recovery.temporary"}],"sessions":[{"state":"failed","resource":"absent","anchor":"present","controller":"absent","cleanup":"unknown","workload_exit":137}]}`, 75)
	w := &world{recovery: &recoveryNative{status: status, stops: []*recoveryProcess{stop}, processes: []*recoveryProcess{status, stop}}}
	w.offline = &offlineRuntime{api: &offlineAPI{requests: []offlineRequest{
		{Method: "GET", Path: "/info", Status: 200, At: start, Replied: start.Add(50 * time.Millisecond)},
		{Method: "GET", Path: "/volumes/private-volume", Status: 502, Cancelled: true, At: start.Add(60 * time.Millisecond), Diagnostic: "recorder transport: context canceled private details"},
	}}}
	_, err := w.anchorsEntry(status, 0)
	if err == nil {
		t.Fatal("unexpected recovery exit passed")
	}
	for _, want := range []string{"anchor recovery exit 75, expected 0", "status exit=75", "recovery.temporary", "resource=running anchor=unknown", "stop-1 exit=75", "resource=absent anchor=present", "GET /info HTTP=200", "GET /volumes/<resource> HTTP=502", "cancelled=true", "elapsed=1s"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %s", want, err)
		}
	}
	if strings.Contains(err.Error(), "private") {
		t.Fatal("diagnostic exposed arbitrary output or resource names", err)
	}
}

func TestAnchorCleanupConflictDiagnostics(t *testing.T) {
	for _, tc := range []struct{ message, reason string }{
		{"volume private-volume is in use - [private-container]", "in-use"},
		{"remove private-path: device or resource busy", "busy"},
		{"private unrecognized engine response", "unclassified"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			deletes := 0
			v := &anchorVolume{name: "private-volume", labels: map[string]any{}, options: map[string]any{}}
			client := &http.Client{Transport: anchorOracleTransport(func(r *http.Request) (*http.Response, error) {
				status, body := 200, `{"Name":"private-volume","Driver":"local","Scope":"local","CreatedAt":"now","Labels":{},"Options":{}}`
				if r.Method == "DELETE" {
					deletes++
					status, body = 409, string(approvalJSON(map[string]any{"message": tc.message}))
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			})}
			w := &world{doctor: &doctorWorld{real: &doctorReal{client: client}}}
			a := &offlineAPI{w: w, volumes: map[string]*anchorVolume{v.name: v}}
			err := a.cleanupAnchorVolumes(t.Context())
			if err == nil || !strings.Contains(err.Error(), "HTTP 409 reason="+tc.reason) || strings.Contains(err.Error(), "private") {
				t.Fatalf("cleanup lost its safe conflict reason: %v", err)
			}
			if deletes != 1 || len(a.requests) != 0 {
				t.Fatalf("diagnosis retried cleanup or added product evidence: deletes=%d requests=%d", deletes, len(a.requests))
			}
		})
	}
}

func TestAnchorRecoveryDiagnosticsExcludeUntrustedFields(t *testing.T) {
	for _, report := range []string{
		"private invalid JSON",
		`{"schema":1,"problems":[{"rule":"private rule"}],"sessions":[{"state":"private state","resource":"private resource","anchor":"private anchor","controller":"private controller","cleanup":"private cleanup"}]}`,
	} {
		p := &recoveryProcess{done: make(chan struct{}), exit: 75}
		_, _ = p.out.Write([]byte(report))
		close(p.done)
		w := &world{recovery: &recoveryNative{status: p, after: &recoveryProcess{done: make(chan struct{})}}}
		w.offline = &offlineRuntime{api: &offlineAPI{requests: []offlineRequest{
			{Method: "private method", Path: "/private-route", Diagnostic: "private diagnostic"},
			{Method: "POST", Path: "/containers/private-id/private-operation", Query: "private-query", Body: map[string]any{"private": "body"}},
		}}}
		_, err := w.anchorsEntry(p, 0)
		if err == nil || !strings.Contains(err.Error(), "status-after pending") || strings.Contains(err.Error(), "private") {
			t.Fatalf("diagnostics lost the failure, blocked, or leaked arbitrary data: %v", err)
		}
	}
}
