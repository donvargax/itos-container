package features

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Capture existing observations before scenario cleanup destroys the world.
// Beyond the wait for its own recorder's open creates, diagnostics never block,
// retry a product command or query the engine. Only known enum values and
// request metadata are emitted, never workload streams, settings, record
// contents, query strings, resource names or raw engine error messages.
func (w *world) anchorRecoveryDiagnostics() string {
	var out strings.Builder
	origin := w.anchorLaunchDiagnostic(&out)
	if r := w.recovery; r != nil {
		anchorProcessDiagnostic(&out, "status", r.status)
		for i, p := range r.stops {
			anchorProcessDiagnostic(&out, fmt.Sprintf("stop-%d", i+1), p)
		}
		anchorProcessDiagnostic(&out, "status-after", r.after)
	}
	if w.offline != nil && w.offline.api != nil {
		a := w.offline.api
		// A printed create always shows its outcome, the engine's answer or the
		// expired allowance, never only how long it had been open.
		a.awaitCreates()
		a.mu.Lock()
		defer a.mu.Unlock()
		// Keep the first recovery requests as well as the tail: the status
		// which failed can precede several complete stop attempts.
		start := 0
		if w.anchors != nil {
			start = min(w.anchors.recoveryIndex, len(a.requests))
		}
		if origin.IsZero() && len(a.requests) > 0 {
			origin = a.requests[0].At
		}
		// Without a recovery checkpoint the listing below starts at the first request.
		anchorLaunchSummary(&out, origin, a.requests[:start])
		fmt.Fprintf(&out, "product requests=%d showing-from=%d\n", len(a.requests), start)
		for i, q := range a.requests[start:] {
			if i >= 32 && i < len(a.requests)-start-32 {
				continue
			}
			anchorRequestDiagnostic(&out, origin, q)
		}
	}
	return strings.TrimSpace(out.String())
}

// The typed problems a launch controller can stop with, and the operations its
// call limit names. Anything else on its stderr prints as unrecognized.
var (
	anchorLaunchProblemLine = regexp.MustCompile(`(?m)^itos-container: (run\.[a-z0-9-]+)([^\n]*)$`)
	anchorLaunchRules       = []string{"run.call-timeout", "run.startup-timeout", "run.teardown-timeout", "run.stream-timeout",
		"run.create-uncertain", "run.cleanup-incomplete", "run.cleanup-temporary", "run.cleanup-ownership", "run.cleanup-internal",
		"run.cleanup-environment", "run.cleanup-configuration", "run.execution-unavailable", "run.engine-lost", "run.cancelled",
		"run.mount-refused", "run.mount-unavailable", "run.mount-uncertain", "run.mount-write", "run.ownership-refused",
		"run.ownership-unsafe", "run.isolation-unverified", "run.isolation-inspection", "run.environment-unavailable",
		"run.engine-environment-missing", "run.checkout-overlap"}
	anchorLaunchOperations = []string{"engine connection", "engine inspection", "session lock acquisition", "session discovery",
		"intent lock acquisition and intent publication", "session registration", "anchor allocation", "anchor inspection",
		"anchor removal", "create response", "boundary inspection", "attach establishment", "start acknowledgement",
		"resource inspection", "resource removal", "signal delivery"}
)

// The first line: the launch controller, its exit and elapsed time or pending,
// with its typed rule and operation. It returns the launch's start, the origin
// of the arrival offsets, when the controller is a recorded process.
func (w *world) anchorLaunchDiagnostic(out *strings.Builder) (origin time.Time) {
	var stderr []byte
	switch {
	case w.recovery != nil && w.recovery.run != nil:
		p := w.recovery.run
		select {
		case <-p.done:
		default:
			fmt.Fprintln(out, "launch pending")
			return p.started
		}
		fmt.Fprintf(out, "launch exit=%d elapsed=%s", p.exit, p.finished.Sub(p.started))
		stderr, origin = p.errout.bytes(), p.started
	case w.recovery == nil && w.offline != nil:
		// The plain anchor fixture's controller is not a recorded process: its exit
		// and stderr are the offline run's, and it has no elapsed time.
		fmt.Fprintf(out, "launch exit=%d", w.offline.exit)
		stderr = w.offline.stderr
	default:
		return origin
	}
	rule, operation := anchorLaunchProblem(stderr)
	fmt.Fprintf(out, " rule=%s operation=%s\n", rule, operation)
	return origin
}

func anchorLaunchProblem(stderr []byte) (rule, operation string) {
	rule, operation = "none", "none"
	// The typed problem is the last thing a controller prints, after any workload
	// output it relayed. Bound what is searched to the end of its stderr.
	const window = 64 << 10
	if len(stderr) > window {
		stderr = stderr[len(stderr)-window:]
	}
	found := anchorLaunchProblemLine.FindSubmatch(stderr)
	if found == nil {
		return rule, operation
	}
	rule = anchorDiagnosticValue(string(found[1]), anchorLaunchRules...)
	if _, named, ok := strings.Cut(string(found[2]), "; operation "); ok {
		named, _, _ = strings.Cut(named, ";")
		operation = anchorDiagnosticValue(named, anchorLaunchOperations...)
	}
	return rule, operation
}

// A bounded summary of the launch requests, before the recovery requests: their
// total, then in the journal's order every create, the request before each
// (the image inspection that opens the product's create call) and every start,
// and every refused, failed, cancelled or unanswered request, twelve at most.
func anchorLaunchSummary(out *strings.Builder, origin time.Time, launch []offlineRequest) {
	if len(launch) == 0 {
		return
	}
	const limit = 12
	selected := map[int]bool{}
	for i, q := range launch {
		create := q.Method == "POST" && q.Path == "/containers/create"
		if create && i > 0 && len(selected) < limit {
			selected[i-1] = true
		}
		if (create || (q.Method == "POST" && strings.HasSuffix(q.Path, "/start"))) && len(selected) < limit {
			selected[i] = true
		}
	}
	for i, q := range launch {
		if len(selected) >= limit {
			break
		}
		if q.Status >= http.StatusBadRequest || q.Status == 0 || q.Cancelled || q.Replied.IsZero() {
			selected[i] = true
		}
	}
	fmt.Fprintf(out, "launch requests=%d omitted=%d\n", len(launch), len(launch)-len(selected))
	for i, q := range launch {
		if selected[i] {
			anchorRequestDiagnostic(out, origin, q)
		}
	}
}

// One request: its route, never its resource; how it ended; when it arrived
// after the launch began; and for a create the three intervals of its call: the
// recorder's own checks and volume inspection before forwarding, the engine's
// time, and the recorder's recording and inspection before the reply.
func anchorRequestDiagnostic(out *strings.Builder, origin time.Time, q offlineRequest) {
	method := anchorDiagnosticValue(q.Method, "GET", "HEAD", "POST", "DELETE")
	fmt.Fprintf(out, "%s %s HTTP=%d cancelled=%t replied=%t reason=%s", method, anchorDiagnosticRoute(q.Path), q.Status, q.Cancelled, !q.Replied.IsZero(), anchorDiagnosticReason(q.Diagnostic))
	if !origin.IsZero() && !q.At.IsZero() {
		fmt.Fprintf(out, " arrival=%s", q.At.Sub(origin))
	}
	out.WriteByte('\n')
	if !q.At.IsZero() && !q.Replied.IsZero() {
		fmt.Fprintf(out, "reply-after=%s\n", q.Replied.Sub(q.At))
	}
	if q.Method == "POST" && q.Path == "/containers/create" {
		fmt.Fprintf(out, "create arrival-to-forward=%s forward-to-answer=%s answer-to-reply=%s\n",
			anchorInterval(q.At, q.Forwarded), anchorInterval(q.Forwarded, q.Answered), anchorInterval(q.Answered, q.Replied))
	}
}

func anchorInterval(from, to time.Time) string {
	if from.IsZero() || to.IsZero() {
		return "missing"
	}
	return to.Sub(from).String()
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
	if path == "/containers/create" {
		return path
	}
	if strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json") {
		return "/images/<resource>/json"
	}
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
	case message == offlineCreateExpired:
		return "create-allowance-expired"
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
