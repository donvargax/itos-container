package disk

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

// countedLogs counts the storage calls a Begin makes.
type countedLogs struct {
	ap.SessionLogs
	calls int
}

func (l *countedLogs) Stage(ctx context.Context, c ap.Candidate, id string) (ap.StagedLog, ap.Failure) {
	l.calls++
	return l.SessionLogs.Stage(ctx, c, id)
}

func TestRefusalLogBeginRefusals(t *testing.T) {
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name  string
		want  sp.Outcome
		calls int
	}{
		{"registered session", "", 1},
		{"session name", sp.Invalid, 0},
		{"63 digits", sp.Invalid, 0},
		{"uppercase digits", sp.Invalid, 0},
		{"relative root", sp.Refused, 0},
		{"refused host identity", sp.Refused, 0},
		{"state.path", sp.Unsafe, 1},
		{"state.unsafe", sp.Unsafe, 1},
		{"state.location", sp.Unsafe, 1},
		{"state.invalid", sp.Invalid, 1},
		{"state.conflict", sp.Refused, 1},
		{"state.unavailable", sp.Incomplete, 1},
		{"state.write", sp.Incomplete, 1},
		{"state.busy", sp.Incomplete, 1},
		{"unsafe session directory", sp.Unsafe, 1},
		{"unregistered session", sp.Refused, 1},
		{"ended context", sp.Incomplete, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := refusalLogFixture()
			logs := &countedLogs{SessionLogs: f.files}
			f.log.Storage = logs
			ctx, session := t.Context(), f.id
			switch tc.name {
			case "session name":
				session = "session"
			case "63 digits":
				session = f.id[1:]
			case "uppercase digits":
				session = strings.ToUpper(f.id)
			case "relative root":
				f.log.Store.Input.Root = "relative/checkout"
			case "refused host identity":
				f.host.UID = "0"
			case "unsafe session directory":
				f.files.Sessions[f.key+"/"+f.id].Unsafe = true
			case "unregistered session":
				session = strings.Repeat("c", 64)
			case "ended context":
				ctx = ended
			case "registered session":
			default:
				f.files.Failure = ap.Failure(tc.name)
			}
			copy, failure := f.log.Begin(ctx, session)
			if copy != nil {
				copy.Close()
			}
			if failure != tc.want || (copy == nil) != (tc.want != "") || logs.calls != tc.calls {
				t.Fatalf("begin = %v, %s after %d storage calls; want %q after %d", copy, failure, logs.calls, tc.want, tc.calls)
			}
			if f.staged() != 0 || f.stored() != nil {
				t.Fatal("a refused or closed copy left a handle or a log")
			}
		})
	}
}

func TestRefusalLogFailuresKeepEarlierCopy(t *testing.T) {
	refusal := strings.SplitAfter(sf.ProxyStdout, "\n")[1]
	next := strings.Replace(refusal, "not-listed", "port", 1)
	for _, tc := range []struct {
		name    string
		fault   string
		want    sp.Outcome
		visible string
	}{
		{"none", "", "", refusal + next},
		{"write", "write", sp.Incomplete, "earlier\n"},
		{"write of the held line", "write", sp.Incomplete, "earlier\n"},
		{"write, then Close with no Commit", "write", sp.Incomplete, "earlier\n"},
		{"file fsync", "file fsync", sp.Incomplete, "earlier\n"},
		{"atomic publication", "atomic publication", sp.Incomplete, "earlier\n"},
		{"parent fsync", "parent fsync", sp.Incomplete, refusal + next},
		{"changed session directory", "", sp.Unsafe, "earlier\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := refusalLogFixture()
			f.copyAll(t, "earlier\n")
			copy := f.begin(t)
			if n, err := copy.Write([]byte(refusal)); n != len(refusal) || err != nil {
				t.Fatalf("write before the fault = %d, %v", n, err)
			}
			f.files.Fault = tc.fault
			second := next
			if tc.name == "write of the held line" {
				// With no newline nothing is stored yet, so the fault meets Commit's own write.
				second = strings.TrimSuffix(next, "\n")
			}
			if tc.name == "changed session directory" {
				f.files.Sessions[f.key+"/"+f.id].Changed = true
			}
			n, err := copy.Write([]byte(second))
			if failed := tc.fault == "write" && second == next; failed {
				if n != 0 || err == nil || err.Error() != "refusal copy unavailable" {
					t.Fatalf("write under the fault = %d, %v", n, err)
				}
			} else if n != len(second) || err != nil {
				t.Fatalf("write = %d, %v", n, err)
			}
			if tc.name != "write, then Close with no Commit" {
				report, failure := copy.Commit()
				if failure != tc.want || report.Outcome != "" || report.Gaps != nil {
					t.Fatalf("commit = %#v, %s; want %q", report, failure, tc.want)
				}
				if f.staged() != 0 {
					t.Fatal("Commit returned before its staged log was released")
				}
			}
			calls := len(f.files.Events)
			copy.Close()
			if got := string(f.stored()); got != tc.visible || f.staged() != 0 || len(f.files.Events) != calls {
				t.Fatalf("after Close the log is %q with %d staged and %d more calls; want %q", got, f.staged(), len(f.files.Events)-calls, tc.visible)
			}
			// The copy has ended: nothing more reaches the staged log.
			if n, err := copy.Write([]byte(refusal)); n != 0 || err == nil || err.Error() != "refusal copy closed" {
				t.Fatalf("write after the copy ended = %d, %v", n, err)
			}
			if report, failure := copy.Commit(); failure != sp.Incomplete || report.Outcome != "" || report.Gaps != nil {
				t.Fatalf("second commit = %#v, %s", report, failure)
			}
			copy.Close()
			if got := string(f.stored()); got != tc.visible || f.staged() != 0 || len(f.files.Events) != calls {
				t.Fatal("an ended copy reached the staged log again")
			}
			f.files.Fault, f.files.Sessions[f.key+"/"+f.id].Changed = "", false
			f.copyAll(t, "retry\n")
			if got := string(f.stored()); got != "retry\n" || f.staged() != 0 {
				t.Fatalf("the next copy stored %q", got)
			}
		})
	}
}

// heldLogs wraps a SessionLogs and holds one call of the next staged log until
// the test releases it: the write or the fsync that a timed-out call leaves
// running. It counts every call that reaches a staged log.
type heldLogs struct {
	ap.SessionLogs
	hold             string
	entered, release chan struct{}
	calls            atomic.Int32
}

func (h *heldLogs) Stage(ctx context.Context, c ap.Candidate, id string) (ap.StagedLog, ap.Failure) {
	staged, failure := h.SessionLogs.Stage(ctx, c, id)
	if staged == nil {
		return nil, failure
	}
	hold := h.hold
	h.hold = ""
	return &heldLog{StagedLog: staged, logs: h, hold: hold}, failure
}

type heldLog struct {
	ap.StagedLog
	logs *heldLogs
	hold string
}

func (l *heldLog) wait(call string) {
	l.logs.calls.Add(1)
	if l.hold == call {
		l.logs.entered <- struct{}{}
		<-l.logs.release
	}
}
func (l *heldLog) Write(data []byte) ap.Failure { l.wait("write"); return l.StagedLog.Write(data) }
func (l *heldLog) Publish() ap.Failure          { l.wait("publish"); return l.StagedLog.Publish() }
func (l *heldLog) Close()                       { l.wait("close"); l.StagedLog.Close() }

// returns fails the test unless fn returns while the staged log's call is held.
func returns(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { fn(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s waited for the held call", what)
	}
}

func TestRefusalLogCloseNeverWaits(t *testing.T) {
	refusal := strings.SplitAfter(sf.ProxyStdout, "\n")[1]
	first, second := strings.Replace(refusal, "not-listed", "port", 1), strings.Replace(refusal, "not-listed", "literal-ip", 1)
	f := refusalLogFixture()
	c := ap.Candidate{Project: f.key}
	held, failure := f.files.Open(t.Context(), c, f.id, false)
	if failure != "" || held == nil {
		t.Fatal(failure)
	}
	sessionHeld := true
	defer func() {
		if sessionHeld {
			held.Close()
		}
	}()
	f.copyAll(t, "earlier\n")
	logs := &heldLogs{SessionLogs: f.files, entered: make(chan struct{}), release: make(chan struct{})}
	f.log.Storage = logs

	// A Write is held inside the staged log, as an uninterruptible write would be.
	logs.hold = "write"
	closed := f.begin(t)
	type written struct {
		n   int
		err error
	}
	wrote := make(chan written)
	go func() { n, err := closed.Write([]byte(refusal)); wrote <- written{n, err} }()
	<-logs.entered
	returns(t, "Close", closed.Close)
	if f.staged() != 1 || logs.calls.Load() != 1 {
		t.Fatal("Close released a staged log that a Write was still using", f.staged(), logs.calls.Load())
	}
	later := make(chan sp.Outcome)
	go func() { _, failure := closed.Commit(); later <- failure }()
	logs.release <- struct{}{}
	if w := <-wrote; w.n != len(refusal) || w.err != nil {
		t.Fatalf("the held Write finished with %d, %v", w.n, w.err)
	}
	if failure := <-later; failure != sp.Incomplete {
		t.Fatal("a Commit after Close published", failure)
	}
	// The Write closed the staged log itself: one Write, one Close, nothing published.
	if got := string(f.stored()); got != "earlier\n" || f.staged() != 0 || logs.calls.Load() != 2 {
		t.Fatalf("after the held Write the log is %q with %d staged and %d calls", got, f.staged(), logs.calls.Load())
	}
	if n, err := closed.Write([]byte(refusal)); n != 0 || err == nil {
		t.Fatal("a Write after Close was accepted", n, err)
	}
	if report, failure := closed.Commit(); failure != sp.Incomplete || report.Outcome != "" || report.Gaps != nil {
		t.Fatalf("a later Commit = %#v, %s", report, failure)
	}
	closed.Close()
	if got := string(f.stored()); got != "earlier\n" || f.staged() != 0 || logs.calls.Load() != 2 {
		t.Fatal("an ended copy reached the staged log again", logs.calls.Load())
	}

	// Close is held inside the release it makes when nothing is running. A Commit
	// and a Write that get their turn meanwhile find the copy ended, and neither
	// returns while its staged log is still open.
	logs.hold = "close"
	ending := f.begin(t)
	released := make(chan struct{})
	go func() { ending.Close(); close(released) }()
	<-logs.entered
	type refused struct {
		call   string
		ok     bool
		staged int
	}
	after := make(chan refused, 2)
	go func() {
		_, failure := ending.Commit()
		after <- refused{"Commit", failure == sp.Incomplete, f.staged()}
	}()
	go func() {
		n, err := ending.Write([]byte(refusal))
		after <- refused{"Write", n == 0 && err != nil, f.staged()}
	}()
	select {
	case got := <-after:
		t.Fatalf("%s returned with %d staged while Close was still releasing the staged log", got.call, got.staged)
	case <-time.After(100 * time.Millisecond):
	}
	logs.release <- struct{}{}
	<-released
	for range 2 {
		if got := <-after; !got.ok || got.staged != 0 {
			t.Fatalf("%s on the ended copy: refused %t with %d staged", got.call, got.ok, got.staged)
		}
	}
	// Close alone reached the staged log, and nothing was published.
	if got := string(f.stored()); got != "earlier\n" || f.staged() != 0 || logs.calls.Load() != 3 {
		t.Fatalf("after the held Close the log is %q with %d staged and %d calls", got, f.staged(), logs.calls.Load())
	}

	// A Commit is held inside Publish, past the point where its call timed out.
	logs.hold = "publish"
	late := f.begin(t)
	if n, err := late.Write([]byte(first)); n != len(first) || err != nil {
		t.Fatal(n, err)
	}
	type committed struct {
		report  sp.RefusalReport
		failure sp.Outcome
	}
	commit := make(chan committed)
	go func() { report, failure := late.Commit(); commit <- committed{report, failure} }()
	<-logs.entered
	returns(t, "Close", late.Close)
	if f.staged() != 1 {
		t.Fatal("Close undid a Commit that had begun")
	}
	// The controller gives the session up while that Publish still runs, and
	// another controller copies the same stopped proxy's log under its own Open.
	held.Close()
	sessionHeld = false
	other, failure := f.files.Open(t.Context(), c, f.id, false)
	if failure != "" || other == nil {
		t.Fatal("the session's lock outlived its holder", failure)
	}
	defer other.Close()
	f.copyAll(t, second)
	if got := string(f.stored()); got != second || f.staged() != 1 {
		t.Fatalf("the second controller stored %q with %d staged", got, f.staged())
	}
	logs.release <- struct{}{}
	if done := <-commit; done.failure != "" || done.report.Outcome != "" || done.report.Gaps != nil {
		t.Fatalf("the late Commit = %#v, %s", done.report, done.failure)
	}
	if got := string(f.stored()); got != first || f.staged() != 0 {
		t.Fatalf("the late publication left %q with %d staged, want the first copy whole", got, f.staged())
	}
	if !slices.Equal(f.operations()[len(f.operations())-3:], []string{"file fsync", "atomic publication", "parent fsync"}) {
		t.Fatal("the late publication was not one whole publication", f.operations())
	}
}
