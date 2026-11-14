package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// T-50: a row's first failure says what it is. The waits run under controlled
// time against temporary report files; the Thens and both journal formatters
// read journals this test writes. No product, native worker or engine runs.
func TestAnchorRecoveryBarrierDiagnostics(t *testing.T) {
	t.Run("waits", func(t *testing.T) {
		for name, test := range map[string]func(*testing.T){
			"controller exits 75 before the first report":      testAnchorWaitEarlyExit,
			"live controller at the limit":                     testAnchorWaitLiveLimit,
			"missed later progress":                            testAnchorWaitMissedProgress,
			"later progress arrives":                           testAnchorWaitProgress,
			"uncertain startup ended by the controller's exit": testAnchorHeldReplyExit,
			"uncertain startup ended by its limit":             testAnchorHeldReplyLimit,
			"uncertain startup reaches the held reply":         testAnchorHeldReplyReached,
			"late first report after the fixture's kill":       testAnchorWaitAfterKill,
			"missing first report after the fixture's kill":    testAnchorWaitAfterKillMissing,
		} {
			t.Run(name, func(t *testing.T) { synctest.Test(t, test) })
		}
	})
	t.Run("first Thens return the recorded cause first", testAnchorCauseFirst)
	t.Run("plain fixture whose create was abandoned", testAnchorPlainAbandoned)
	t.Run("launch line", testAnchorLaunchLine)
	t.Run("oversized and private input", testAnchorOversizedJournal)
	t.Run("lifecycle journal prints an abandoned create", testLifecycleAbandonedCreate)
	t.Run("formatters wait for an open create", func(t *testing.T) {
		for _, kind := range []string{"recovery", "lifecycle"} {
			t.Run(kind, func(t *testing.T) { synctest.Test(t, func(t *testing.T) { testFormatterAwaitsCreate(t, kind) }) })
		}
	})
}

func anchorWaitWorld(t *testing.T) *world {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	return &world{anchor: &anchorWorld{original: dir},
		anchors:  &anchorRecoveryWorld{mode: "start", ready: make(chan struct{}), release: make(chan struct{})},
		recovery: &recoveryNative{run: &recoveryProcess{exit: -1, done: make(chan struct{}), started: time.Now()}}}
}

func anchorWaitReport(t *testing.T, w *world, sequence int) {
	t.Helper()
	path := filepath.Join(w.anchor.original, "subdir", "slice-17-recovery-report")
	if err := os.WriteFile(path+".next", fmt.Appendf(nil, `{"Sequence":%d}`, sequence), 0600); err != nil {
		t.Error(err)
		return
	}
	if err := os.Rename(path+".next", path); err != nil {
		t.Error(err)
	}
}

func anchorWaitExit(p *recoveryProcess, code int) {
	p.exit, p.finished = code, time.Now()
	close(p.done)
}

// An event of the fixture after a delay of the bubble's clock. The returned
// join is deferred: a bubble may not end while its goroutines still sleep.
func anchorWaitLater(delay time.Duration, event func()) (join func()) {
	done := make(chan struct{})
	go func() { defer close(done); time.Sleep(delay); event() }()
	return func() { <-done }
}

func requireWords(t *testing.T, err error, words ...string) {
	t.Helper()
	if err == nil {
		t.Error("a wait that ended without its report recorded nothing")
		return
	}
	for _, word := range words {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("recorded cause omits %q: %v", word, err)
		}
	}
}

// One 20 ms poll may pass between an event and the wait's reading of it.
func requireElapsed(t *testing.T, started time.Time, want time.Duration) {
	t.Helper()
	if elapsed := time.Since(started); elapsed < want || elapsed > want+20*time.Millisecond {
		t.Errorf("the wait ended after %s, want %s", elapsed, want)
	}
}

func testAnchorWaitEarlyExit(t *testing.T) {
	w := anchorWaitWorld(t)
	p, started := w.recovery.run, time.Now()
	defer anchorWaitLater(1200*time.Millisecond, func() { anchorWaitExit(p, 75) })()
	if report := w.anchorsAwait(0); report.Sequence != 0 {
		t.Errorf("a controller that exited produced report %d", report.Sequence)
	}
	requireElapsed(t, started, 1200*time.Millisecond)
	requireWords(t, w.recovery.err, "first probe report", "controller exited", "elapsed=1.2", "limit=35s", "launch controller exit=75 after 1.2s")
}

func testAnchorWaitLiveLimit(t *testing.T) {
	w := anchorWaitWorld(t)
	started := time.Now()
	if report := w.anchorsAwait(0); report.Sequence != 0 {
		t.Errorf("a silent worker produced report %d", report.Sequence)
	}
	requireElapsed(t, started, 35*time.Second)
	requireWords(t, w.recovery.err, "first probe report", "limit expired with live controller", "elapsed=35s", "limit=35s", "launch controller pending")
}

func testAnchorWaitMissedProgress(t *testing.T) {
	w := anchorWaitWorld(t)
	anchorWaitReport(t, w, 3)
	w.anchors.killed = true
	anchorWaitExit(w.recovery.run, -1)
	started := time.Now()
	if report := w.anchorsProgress(); report.Sequence != 0 {
		t.Errorf("a stalled worker produced report %d", report.Sequence)
	}
	requireElapsed(t, started, 35*time.Second)
	requireWords(t, w.recovery.err, "later probe progress", "limit expired", "elapsed=35s", "limit=35s")
	if w.recovery.err != nil && strings.Contains(w.recovery.err.Error(), "first probe report") {
		t.Errorf("a later-progress miss was recorded as a first-report miss: %v", w.recovery.err)
	}
}

func testAnchorWaitProgress(t *testing.T) {
	w := anchorWaitWorld(t)
	anchorWaitReport(t, w, 3)
	w.anchors.killed = true
	anchorWaitExit(w.recovery.run, -1)
	started := time.Now()
	defer anchorWaitLater(time.Second, func() { anchorWaitReport(t, w, 4) })()
	if report := w.anchorsProgress(); report.Sequence != 4 || w.recovery.err != nil {
		t.Errorf("later progress = report %d, recorded %v", report.Sequence, w.recovery.err)
	}
	requireElapsed(t, started, time.Second)
}

func testAnchorHeldReplyExit(t *testing.T) {
	w := anchorWaitWorld(t)
	p, started := w.recovery.run, time.Now()
	defer anchorWaitLater(900*time.Millisecond, func() { anchorWaitExit(p, 75) })()
	if w.anchorsHeldReply() || w.anchors.reached {
		t.Error("a controller that exited reached the held reply")
	}
	requireElapsed(t, started, 900*time.Millisecond)
	requireWords(t, w.recovery.err, "held reply", "controller exited", "elapsed=900ms", "limit=35s", "launch controller exit=75 after 900ms")
}

func testAnchorHeldReplyLimit(t *testing.T) {
	w := anchorWaitWorld(t)
	started := time.Now()
	if w.anchorsHeldReply() || w.anchors.reached {
		t.Error("a missing barrier counted as reached")
	}
	requireElapsed(t, started, 35*time.Second)
	requireWords(t, w.recovery.err, "held reply", "limit expired with live controller", "elapsed=35s", "limit=35s", "launch controller pending")
}

func testAnchorHeldReplyReached(t *testing.T) {
	w := anchorWaitWorld(t)
	started := time.Now()
	defer anchorWaitLater(500*time.Millisecond, func() { close(w.anchors.ready) })()
	if !w.anchorsHeldReply() || !w.anchors.reached || w.recovery.err != nil {
		t.Errorf("a held reply was not reached: reached=%t recorded %v", w.anchors.reached, w.recovery.err)
	}
	requireElapsed(t, started, 500*time.Millisecond)
}

// After the fixture has itself killed the controller, the dead controller ends
// no wait: a worker slow to write its first report is still heard.
func testAnchorWaitAfterKill(t *testing.T) {
	w := anchorWaitWorld(t)
	w.anchors.killed = true
	anchorWaitExit(w.recovery.run, -1)
	started := time.Now()
	defer anchorWaitLater(2*time.Second, func() { anchorWaitReport(t, w, 1) })()
	if report := w.anchorsAwait(0); report.Sequence != 1 || w.recovery.err != nil {
		t.Errorf("late first report after the fixture's kill = report %d, recorded %v", report.Sequence, w.recovery.err)
	}
	requireElapsed(t, started, 2*time.Second)
}

func testAnchorWaitAfterKillMissing(t *testing.T) {
	w := anchorWaitWorld(t)
	w.anchors.killed = true
	anchorWaitExit(w.recovery.run, -1)
	started := time.Now()
	if report := w.anchorsAwait(0); report.Sequence != 0 {
		t.Errorf("a missing first report produced report %d", report.Sequence)
	}
	requireElapsed(t, started, 35*time.Second)
	requireWords(t, w.recovery.err, "first probe report", "limit expired after the fixture killed the controller", "elapsed=35s", "limit=35s")
}

const anchorPrivate = "SYNTHETIC_PRIVATE_FIELD"

// A recovery fixture whose launch controller stopped at its create call: the
// journal of that launch, then two requests of fresh recovery.
func anchorCauseWorld(mode string, exit int, cause error) *world {
	start := time.Unix(100, 0)
	launch := anchorLaunchProcess(exit, anchorPrivate+" workload output\nitos-container: run.call-timeout; operation create response\n")
	at := func(ms int) time.Time { return start.Add(time.Duration(ms) * time.Millisecond) }
	private := map[string]any{"Env": anchorPrivate}
	id := strings.Repeat("a", 64)
	requests := []offlineRequest{
		{Method: "GET", Path: "/_ping", Status: 200, At: at(10), Replied: at(11)},
		{Method: "GET", Path: "/info", Status: 200, At: at(20), Replied: at(25), Observation: private},
		{Method: "POST", Path: "/volumes/create", Status: 201, At: at(40), Replied: at(60), Body: private, Record: sessionRecord{Bytes: anchorPrivate}},
		{Method: "GET", Path: "/images/sha256:" + anchorPrivate + "/json", Status: 200, At: at(100), Replied: at(130), Observation: private},
		{Method: "POST", Path: "/containers/create", Query: "name=" + anchorPrivate, Body: private, Anchor: private, Status: 201, Cancelled: true,
			At: at(140), Forwarded: at(145), Answered: at(1240)},
		{Method: "GET", Path: "/containers/itos-container-" + anchorPrivate + "/json", Status: 200, At: at(1250), Replied: at(1260), Observation: private},
		{Method: "GET", Path: "/containers/" + id + "/json", Status: 403, At: at(1270), Diagnostic: "recorder refused unsafe request"},
		{Method: "GET", Path: "/info", Status: 200, At: at(2000), Replied: at(2005)},
		{Method: "GET", Path: "/containers/" + id + "/json", Status: 403, At: at(2010), Diagnostic: "recorder refused unsafe request"},
	}
	return &world{recovery: &recoveryNative{run: launch, err: cause, processes: []*recoveryProcess{launch}},
		anchors: &anchorRecoveryWorld{mode: mode, recoveryIndex: 7}, anchor: &anchorWorld{},
		offline: &offlineRuntime{api: &offlineAPI{requests: requests}}}
}

// A collected launch controller that ran for 1.2 seconds.
func anchorLaunchProcess(exit int, stderr string) *recoveryProcess {
	start := time.Unix(100, 0)
	p := &recoveryProcess{done: make(chan struct{}), exit: exit, started: start, finished: start.Add(1200 * time.Millisecond)}
	_, _ = p.errout.Write([]byte(stderr))
	close(p.done)
	return p
}

func requireCauseFirst(t *testing.T, name string, err, cause error, exit int, bare ...string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: a recorded cause passed", name)
		return
	}
	text := err.Error()
	if !errors.Is(err, cause) || !strings.HasPrefix(text, cause.Error()) {
		t.Errorf("%s: the recorded cause is not first or not found by errors.Is: %s", name, text)
	}
	for _, want := range []string{
		fmt.Sprintf("launch exit=%d elapsed=1.2s rule=run.call-timeout operation=create response", exit),
		"launch requests=7 omitted=4",
		"GET /images/<resource>/json HTTP=200",
		"reply-after=30ms",
		"POST /containers/create HTTP=201 cancelled=true replied=false",
		"create arrival-to-forward=5ms forward-to-answer=1.095s answer-to-reply=missing",
		"GET /containers/<resource>/json HTTP=403",
		"product requests=9 showing-from=7",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%s: diagnostics omit %q:\n%s", name, want, text)
		}
	}
	for _, refused := range append(bare, anchorPrivate, strings.Repeat("a", 64)) {
		if strings.Contains(text, refused) {
			t.Errorf("%s: diagnostics print %q:\n%s", name, refused, text)
		}
	}
}

func testAnchorCauseFirst(t *testing.T) {
	const unreached = "uncertain startup never reached its successful native response barrier"
	const overlap = "two stops did not overlap at the original anchor release"
	for _, tc := range []struct {
		name, mode string
		then       func(*world) error
		bare       string
	}{
		{"anchorsLostStatus", "loss", (*world).anchorsLostStatus, ""},
		{"anchorsCompleted", "retry", (*world).anchorsCompleted, ""},
		{"anchorsRelease in create mode", "create", (*world).anchorsRelease, unreached},
		{"anchorsRelease in start mode", "start", (*world).anchorsRelease, unreached},
		{"anchorsForeignRefused", "foreign", (*world).anchorsForeignRefused, "product never reached owned anchor release"},
		{"anchorsPairCompleted with the cause on A", "pair", (*world).anchorsPairCompleted, overlap},
	} {
		cause := errors.New("synthetic recorded cause of " + tc.name)
		w := anchorCauseWorld(tc.mode, 75, cause)
		w.recovery.second = anchorCauseWorld("control", 0, nil)
		requireCauseFirst(t, tc.name, tc.then(w), cause, 75, tc.bare)
	}
	// B's cause comes after A's and before A's bare precondition, with the
	// diagnostics of the world it belongs to.
	cause := errors.New("synthetic recorded cause of B")
	w := anchorCauseWorld("pair", 0, nil)
	w.recovery.second = anchorCauseWorld("control", 70, cause)
	requireCauseFirst(t, "anchorsPairCompleted with the cause on B", w.anchorsPairCompleted(), cause, 70, overlap)
}

// The plain anchor fixture of ID-OFFLINE-29 and ID-OFFLINE-30, its create
// abandoned through the recorder itself and observed as anchorExecute does.
func testAnchorPlainAbandoned(t *testing.T) {
	for _, tc := range []struct {
		name string
		then func(*world) error
	}{{"anchorOriginal", (*world).anchorOriginal}, {"anchorRetained", (*world).anchorRetained}} {
		w, e := createRecorderWorld(t, "plain")
		w.anchor = &anchorWorld{volumes: map[string]*anchorVolume{}}
		a := w.offline.api
		startCreate(t, w, e, createTokens[0]).abandon(t)
		w.offline.exit, w.offline.stderr = 75, []byte("itos-container: run.call-timeout; operation create response\n")
		w.anchor.err = errors.Join(w.anchor.err, a.observeAfter())
		e.finish()
		w.awaitCreateOutcomes() // the step wait between the When and the first Then
		err := tc.then(w)
		if err == nil {
			t.Errorf("%s passed an abandoned create", tc.name)
			continue
		}
		for _, want := range []string{createAbandonedWords, "launch exit=75 rule=run.call-timeout operation=create response",
			"POST /containers/create HTTP=201 cancelled=true replied=false", "create arrival-to-forward=", "answer-to-reply=missing"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s omits %q:\n%v", tc.name, want, err)
			}
		}
		for _, refused := range []string{"run did not reach the submitted-start barrier",
			"missing original owned volume and protected recovery evidence after controller exit",
			createTransportWords, "elapsed=", "private", createTokens[0]} {
			if strings.Contains(err.Error(), refused) {
				t.Errorf("%s prints %q:\n%v", tc.name, refused, err)
			}
		}
	}
}

func testAnchorLaunchLine(t *testing.T) {
	const rule = "itos-container: run.call-timeout; operation create response\n"
	noise := strings.Repeat(anchorPrivate+"\n", 100000)
	for _, tc := range []struct{ name, stderr, want string }{
		{"no typed problem", "", "rule=none operation=none"},
		{"another listed rule and operation", "itos-container: run.startup-timeout; operation start acknowledgement\n", "rule=run.startup-timeout operation=start acknowledgement"},
		{"a rule without an operation", "itos-container: run.create-uncertain \"intents/" + anchorPrivate + "\": create-uncertain\n", "rule=run.create-uncertain operation=none"},
		{"unlisted values", "itos-container: run.private-rule; operation private operation\n", "rule=unrecognized operation=unrecognized"},
		{"the problem after oversized workload output", noise + rule, "rule=run.call-timeout operation=create response"},
		{"oversized output after the problem", rule + noise, "rule=none operation=none"},
	} {
		w := anchorCauseWorld("loss", 75, nil)
		w.recovery.run = anchorLaunchProcess(75, tc.stderr)
		first, _, _ := strings.Cut(w.anchorRecoveryDiagnostics(), "\n")
		if want := "launch exit=75 elapsed=1.2s " + tc.want; first != want {
			t.Errorf("%s: launch line = %q, want %q", tc.name, first, want)
		}
	}
	w := anchorCauseWorld("loss", 75, nil)
	w.recovery.run = &recoveryProcess{done: make(chan struct{}), started: time.Unix(100, 0)}
	if first, _, _ := strings.Cut(w.anchorRecoveryDiagnostics(), "\n"); first != "launch pending" {
		t.Errorf("uncollected launch controller printed %q", first)
	}
}

func testAnchorOversizedJournal(t *testing.T) {
	w := anchorCauseWorld("loss", 75, nil)
	a := w.offline.api
	start := time.Unix(100, 0)
	var launch []offlineRequest
	for i := range 100 {
		q := offlineRequest{Method: "GET", Path: "/containers/" + anchorPrivate + "/json", Query: anchorPrivate, Status: 403,
			Diagnostic: strings.Repeat(anchorPrivate, 100), Body: map[string]any{"Env": anchorPrivate}, At: start.Add(time.Duration(i) * time.Millisecond)}
		switch i {
		case 59, 89:
			q.Method, q.Path, q.Status, q.Diagnostic = "GET", "/images/"+anchorPrivate+"/json", 200, ""
			q.Replied = q.At.Add(time.Millisecond)
		case 60, 90:
			q.Method, q.Path, q.Status, q.Diagnostic = "POST", "/containers/create", 201, ""
			q.Forwarded, q.Answered, q.Replied = q.At.Add(time.Millisecond), q.At.Add(2*time.Millisecond), q.At.Add(3*time.Millisecond)
		case 95:
			q.Method, q.Path, q.Status, q.Diagnostic = "POST", "/containers/"+anchorPrivate+"/start", 204, ""
			q.Replied = q.At.Add(time.Millisecond)
		}
		launch = append(launch, q)
	}
	a.requests = append(launch, a.requests[7:]...)
	w.anchors.recoveryIndex = 100
	text := w.anchorRecoveryDiagnostics()
	summary, _, found := strings.Cut(text, "product requests=")
	_, summary, cut := strings.Cut(summary, "launch requests=")
	if !found || !cut || !strings.HasPrefix(summary, "100 omitted=88\n") || strings.Count(summary, " HTTP=") != 12 {
		t.Errorf("launch summary is not bounded to twelve of its hundred requests:\n%s", text)
	}
	// Every create, the image inspection before each and the start outrank the
	// refusals around them, and the order of the journal is kept.
	for _, want := range []string{"GET /images/<resource>/json HTTP=200", "POST /containers/create HTTP=201 cancelled=false replied=true",
		"create arrival-to-forward=1ms forward-to-answer=1ms answer-to-reply=1ms", "POST /containers/<resource>/start HTTP=204"} {
		if !strings.Contains(summary, want) {
			t.Errorf("launch summary omits %q:\n%s", want, summary)
		}
	}
	if strings.Count(summary, "POST /containers/create HTTP=201") != 2 || strings.Count(summary, "GET /images/<resource>/json") != 2 ||
		strings.Index(summary, "/containers/create") > strings.Index(summary, "/start") {
		t.Errorf("launch summary lost a create, the request before it or the journal's order:\n%s", summary)
	}
	if strings.Contains(text, anchorPrivate) || len(text) > 20000 {
		t.Errorf("diagnostics exposed a private field or were unbounded (%d bytes)", len(text))
	}
}

// T-49's lifecycle formatter, given an abandoned create: it prints when the
// create was forwarded and when the engine answered, and its selection keeps
// every create and the request before it.
func testLifecycleAbandonedCreate(t *testing.T) {
	origin := time.Now()
	p := &recoveryProcess{exit: 75, started: origin, finished: origin.Add(1200 * time.Millisecond), done: make(chan struct{})}
	_, _ = p.errout.Write([]byte("itos-container: run.call-timeout; operation create response\n"))
	close(p.done)
	a := &offlineAPI{}
	for i := range 100 {
		at := origin.Add(time.Duration(i) * time.Millisecond)
		a.requests = append(a.requests, offlineRequest{Method: "GET", Path: "/info", Query: anchorPrivate, Body: map[string]any{"Env": anchorPrivate},
			Observation: map[string]any{"Config": anchorPrivate}, Record: sessionRecord{Bytes: anchorPrivate}, Status: 200, At: at, Replied: at.Add(time.Millisecond)})
	}
	a.requests[9].Path = "/images/sha256:image/json"
	a.requests[10] = offlineRequest{Method: "POST", Path: "/containers/create", Query: "name=" + anchorPrivate, Body: map[string]any{"Env": anchorPrivate},
		Status: 201, Cancelled: true, At: origin.Add(140 * time.Millisecond), Forwarded: origin.Add(145 * time.Millisecond), Answered: origin.Add(1240 * time.Millisecond)}
	a.requests[49].Path = "/images/sha256:second/json"
	a.requests[50] = offlineRequest{Method: "POST", Path: "/containers/create", Status: 201, At: origin.Add(150 * time.Millisecond),
		Forwarded: origin.Add(151 * time.Millisecond), Answered: origin.Add(152 * time.Millisecond), Replied: origin.Add(153 * time.Millisecond)}
	w := &world{lifecycle: &lifecycleNative{mode: "handle", err: errors.New("product did not reach the handle lifecycle barrier"), run: p}, offline: &offlineRuntime{api: a}}
	err := w.lifecycleResult(17)
	lifecycleRequireDiagnostic(t, err, "product did not reach the handle lifecycle barrier", "exit 75", "run.call-timeout", "total=100", "omitted=88",
		`#10 "GET" "/images/sha256:image/json" HTTP 200`,
		`#11 "POST" "/containers/create" HTTP 201 cancelled=true arrival=140ms reply=missing diagnostic="" forwarded=145ms answer=1.24s`,
		`#50 "GET" "/images/sha256:second/json" HTTP 200`,
		`#51 "POST" "/containers/create" HTTP 201 cancelled=false arrival=150ms reply=153ms diagnostic="" forwarded=151ms answer=152ms`)
	if err != nil && (strings.Contains(err.Error(), anchorPrivate) || strings.Count(err.Error(), "\n#") != 12 ||
		strings.Index(err.Error(), "#11 ") > strings.Index(err.Error(), "#51 ")) {
		t.Errorf("lifecycle journal exposed a private field, lost its order or its limit of twelve:\n%v", err)
	}
	// A request that is no create prints no forwarding or answer time.
	if err != nil && strings.Count(err.Error(), "forwarded=") != 2 {
		t.Errorf("lifecycle journal printed create times for another request:\n%v", err)
	}
}

// A create still open when a formatter is called: the formatter waits for its
// own recorder and prints the outcome, never how long the create had been open.
func testFormatterAwaitsCreate(t *testing.T, kind string) {
	w, e := createRecorderWorld(t, kind)
	started := time.Now()
	launch := &recoveryProcess{exit: 75, started: started, finished: started, done: make(chan struct{})}
	close(launch.done)
	w.recovery.run = launch
	format := w.anchorRecoveryDiagnostics
	want := "POST /containers/create HTTP=201 cancelled=true replied=false"
	if kind == "lifecycle" {
		w.lifecycle.run = launch
		format = func() string { return w.lifecycleResult(17).Error() }
		want = `"POST" "/containers/create" HTTP 201 cancelled=true`
	}
	startCreate(t, w, e, createTokens[0]).abandon(t)
	printed := make(chan string, 1)
	go func() { printed <- format() }()
	synctest.Wait()
	select {
	case text := <-printed:
		t.Fatalf("the formatter printed an open exchange:\n%s", text)
	default:
	}
	time.Sleep(3 * time.Second) // the engine answers late, inside the allowance
	e.finish()
	text := <-printed
	if elapsed := time.Since(started); elapsed != 3*time.Second {
		t.Errorf("the formatter returned after %s, want the exchange's 3s", elapsed)
	}
	outcome := []string{want, "create arrival-to-forward=0s forward-to-answer=3s answer-to-reply=missing"}
	if kind == "lifecycle" {
		outcome = []string{want + " arrival=0s reply=missing diagnostic=\"\" forwarded=0s answer=3s"}
	}
	for _, line := range outcome {
		if !strings.Contains(text, line) {
			t.Errorf("the formatter did not print the create's outcome %q:\n%s", line, text)
		}
	}
}
