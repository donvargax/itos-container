package features

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cucumber/godog"
)

func TestLifecycleGraceObservationPreservesDelayedEvidence(t *testing.T) {
	for _, delay := range []time.Duration{0, 5 * time.Second, 11 * time.Second} {
		t.Run(delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				p := &recoveryProcess{done: make(chan struct{})}
				go func() { time.Sleep(engineGraceBudget); close(p.done) }()
				var signals []string
				var times []time.Duration
				alive := lifecycleRepeatedSignals(p, start, func(signal string) {
					signals = append(signals, signal)
					times = append(times, time.Since(start))
				}, func() bool {
					// Observe the worker before its allowed escalation, but delay
					// the reply beyond it. The observation must remain valid.
					observed := time.Since(start) < engineGraceBudget
					time.Sleep(delay)
					return observed
				})
				if !alive || !slices.Equal(signals, []string{"SIGINT", "SIGTERM"}) || !slices.Equal(times, []time.Duration{engineGraceBudget / 5, 4 * engineGraceBudget / 5}) {
					t.Errorf("delayed inspect lost liveness or moved repetitions: %v %v %v", alive, signals, times)
				}
				<-p.done
			})
		})
	}
}

func TestLifecycleGraceObservationRejectsMissingWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		p := &recoveryProcess{done: make(chan struct{})}
		close(p.done)
		alive := lifecycleRepeatedSignals(p, start, func(string) {
			t.Error("repeated signal after controller exit")
		}, func() bool { return false })
		if alive || time.Since(start) != 0 {
			t.Fatal("missing worker was accepted or early controller exit was ignored")
		}
	})
}

// The rows of LIFECYCLE-09 come from the selection the engine step makes, with the
// environment entering as data, so this runs on any host and reaches no engine.
var lifecycleWindowRows = []struct {
	signal  string
	percent int
}{{"SIGINT", 0}, {"SIGTERM", 0}, {"SIGINT", 60}, {"SIGTERM", 60}}

// One journal of a row, built by hand: the first signal at first after
// cancellation, the product's own waits at waits (it makes one the moment its
// signal call returns), then each of kills, SIGKILL unless repeat names the first
// signal. Each signal's inspection, daemon check and kill reach the recorder at
// one instant, except that the first signal's daemon check is checkLead before the
// kill and its inspection inspectLead before the daemon check, as they are when
// the kill arrives after the call has started. refuses is empty where the journal
// is accepted, else the text the error names.
type lifecycleWindowCase struct {
	name        string
	first       time.Duration
	checkLead   time.Duration
	inspectLead time.Duration
	waits       []time.Duration
	kills       []time.Duration
	repeat      bool
	ack         lifecycleWindowAck
	refuses     string
}

type lifecycleWindowAck int

const (
	ackHeld    lifecycleWindowAck = iota // replied after the row's delay
	ackCut                               // cancelled after the engine accepted it: 204, no reply
	ackStalled                           // cancelled before the engine replied: 502, no reply
	ackEarly                             // replied 100 ms after arrival
	ackNone                              // neither replied nor cancelled
)

func TestLifecycleSignalWindow(t *testing.T) {
	selection, err := selectScenarios("^@ID-LIFECYCLE-09$", ".",
		featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if !selection.engine || selection.skip != "" || selection.count != 4 || !slices.Equal(selection.ids, []string{"@ID-LIFECYCLE-09"}) {
		t.Fatalf("the engine step does not select LIFECYCLE-09's four rows: engine=%t skip=%q count=%d ids=%v",
			selection.engine, selection.skip, selection.count, selection.ids)
	}
	// What runScenarios hands godog: the pickles under the selection's own filter.
	features, err := (&godog.TestSuite{Options: &godog.Options{Paths: []string{"."}, Tags: selection.filter}}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	delayed := regexp.MustCompile(`signal acknowledgement delayed (\d+) percent of grace$`)
	sent := regexp.MustCompile(`^run launches the probe and the harness sends "([^"]*)" followed by repeated SIGINT and SIGTERM during grace$`)
	var read []string
	for _, feature := range features {
		for _, pickle := range feature.Pickles {
			signal, percent := "", ""
			for _, step := range pickle.Steps {
				if m := delayed.FindStringSubmatch(step.Text); m != nil {
					percent = m[1]
				}
				if m := sent.FindStringSubmatch(step.Text); m != nil {
					signal = m[1]
				}
			}
			read = append(read, signal+" "+percent)
		}
	}
	want := []string{"SIGINT 0", "SIGTERM 0", "SIGINT 60", "SIGTERM 60"}
	if !slices.Equal(read, want) {
		t.Fatalf("the selected rows read %q, expected %q", read, want)
	}

	for _, row := range lifecycleWindowRows {
		t.Run(fmt.Sprintf("%s_%d", row.signal, row.percent), func(t *testing.T) {
			delay := time.Duration(row.percent) * engineGraceBudget / 100
			cases := lifecycleImmediateWindowCases()
			if row.percent != 0 {
				cases = lifecycleDelayedWindowCases(delay)
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					w := lifecycleWindowWorld(t, row.signal, delay, c)
					err := w.lifecycleSignals(true)
					switch {
					case c.refuses == "" && err != nil:
						t.Errorf("accepted journal was refused: %v", err)
					case c.refuses != "" && err == nil:
						t.Errorf("journal that must be refused naming %q was accepted", c.refuses)
					case c.refuses != "" && !strings.Contains(err.Error(), c.refuses):
						t.Errorf("journal was refused, but not naming %q: %v", c.refuses, err)
					}
				})
			}
		})
	}

	t.Run("recorder journals a cut acknowledgement", func(t *testing.T) {
		w, mutations := recoveryRecorderWorld(t)
		a := w.offline.api
		a.lifecycle = newLifecycleFaults("ignore", "", time.Hour)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		request := httptest.NewRequest("POST", "http://engine/containers/"+a.created[0]+"/kill?signal=SIGTERM", nil).WithContext(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			a.serve(httptest.NewRecorder(), request)
		}()
		select {
		case <-mutations:
		case <-time.After(10 * time.Second):
			t.Fatal("the daemon never took the kill")
		}
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
			a.mu.Lock()
			set := len(a.requests) == 1 && a.requests[0].Status != 0
			a.mu.Unlock()
			if set {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the journal never recorded the daemon's reply")
			}
		}
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("the recorder kept holding a cancelled acknowledgement")
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if e := a.requests[0]; e.Status != http.StatusNoContent || !e.Cancelled || !e.Replied.IsZero() {
			t.Fatalf("cut acknowledgement journaled Status %d, Cancelled %t, Replied %v; expected 204, true, zero", e.Status, e.Cancelled, e.Replied)
		}
	})
}

func lifecycleImmediateWindowCases() []lifecycleWindowCase {
	const first = 50 * time.Millisecond
	grace := engineGraceBudget
	// The last repeat goes at 80 percent of grace, so a timer it restarted fires
	// no earlier than that plus the grace.
	restart := 4*grace/5 + grace
	return []lifecycleWindowCase{
		{name: "immediate at the grace", first: first, kills: []time.Duration{grace}},
		{name: "immediate past the old window", first: first, kills: []time.Duration{grace + grace/5 + time.Millisecond}},
		{name: "immediate just before a repeat restart", first: first, kills: []time.Duration{restart - time.Millisecond}},
		{name: "immediate before the grace", first: first, kills: []time.Duration{grace - time.Millisecond}, refuses: "SIGKILL at"},
		{name: "immediate at a repeat restart", first: first, kills: []time.Duration{restart}, refuses: "SIGKILL at"},
		{name: "no SIGKILL", first: first, refuses: "signal request count"},
		{name: "second SIGKILL", first: first, kills: []time.Duration{grace, grace + 100*time.Millisecond}, refuses: "signal request count"},
		{name: "first signal repeated instead of SIGKILL", first: first, kills: []time.Duration{grace}, repeat: true, refuses: "literal first signal"},
	}
}

func lifecycleDelayedWindowCases(delay time.Duration) []lifecycleWindowCase {
	const first = 50 * time.Millisecond
	const tolerance = 2 * time.Second // absolute, as the teardown assertion's
	grace := engineGraceBudget
	ms := time.Millisecond
	call := engineCallBudget
	// An acknowledgement-relative timer begins after the hold, which begins after
	// the first signal's arrival.
	restart := func(arrival time.Duration) time.Duration { return arrival + delay + grace }
	return []lifecycleWindowCase{
		{name: "delayed slow teardown", first: 500 * ms, kills: []time.Duration{grace + 300*ms}},
		{name: "delayed just before an acknowledgement restart", first: 300 * ms, kills: []time.Duration{restart(300*ms) - ms}},
		{name: "delayed at the absolute tolerance", first: 1500 * ms, kills: []time.Duration{grace + tolerance}},
		{name: "delayed acknowledgement cut by the call limit", first: first, kills: []time.Duration{grace + 100*ms}, ack: ackCut},
		{name: "delayed before the grace", first: first, kills: []time.Duration{grace - ms}, refuses: "SIGKILL at"},
		{name: "delayed at an acknowledgement restart", first: 300 * ms, kills: []time.Duration{restart(300 * ms)}, refuses: "SIGKILL at"},
		{name: "delayed as an acknowledgement timer on a hosted runner", first: 500 * ms, kills: []time.Duration{restart(500*ms) + 100*ms}, refuses: "SIGKILL at"},
		{name: "delayed past the absolute tolerance", first: 1500 * ms, kills: []time.Duration{grace + tolerance + ms}, refuses: "SIGKILL at"},
		// A late first signal leaves the product's call limit running from the
		// request before the signal, not from cancellation. The cut is journaled
		// nowhere, so the next request decides. An honest cut is followed by the
		// SIGKILL's inspection, because the call limit equals the grace and the
		// call starts after teardown does: the grace deadline has passed by the
		// cut, and the product's wait returns without a request. At the call limit
		// after the request before the signal that inspection is accepted, any
		// sooner it is the product giving up; and a wait is refused whenever it
		// comes, since only a restarted or abandoned grace sends one.
		{name: "delayed acknowledgement cut by the call limit after a late first signal", first: 600 * ms,
			kills: []time.Duration{600*ms + call + 50*ms}, ack: ackCut},
		{name: "delayed hold cut at the call limit after a late first signal", first: 600 * ms, waits: []time.Duration{600*ms + call},
			kills: []time.Duration{600*ms + call + 100*ms}, ack: ackCut, refuses: "delay did not execute"},
		{name: "delayed cut at an acknowledgement restart", first: first, kills: []time.Duration{restart(first)}, ack: ackCut, refuses: "SIGKILL at"},
		{name: "delayed cut before the grace", first: first, kills: []time.Duration{grace - ms}, ack: ackCut, refuses: "SIGKILL at"},
		// The cut is not journaled, but the product's next request follows it, so
		// that request cannot come before the call limit has run out. Here the
		// product gave up 50 ms before the call limit and went straight to its
		// SIGKILL; the same journal with a wait at 150 ms is refused as a wait.
		{name: "delayed hold abandoned before the call limit", first: first,
			kills: []time.Duration{grace}, ack: ackCut, refuses: "delay did not execute"},
		{name: "delayed hold abandoned, then a timer from the abandonment", first: first, waits: []time.Duration{first + 100*ms},
			kills: []time.Duration{first + 100*ms + grace}, ack: ackCut, refuses: "delay did not execute"},
		// The same abandonment after a first signal that arrived late. Measured
		// from cancellation the product's next request is a call limit on, so the
		// hold looks as if it ran; measured from the request before the signal it
		// is not.
		{name: "delayed hold abandoned after a late first signal", first: 1200 * ms,
			kills: []time.Duration{1250 * ms}, ack: ackCut, refuses: "delay did not execute"},
		{name: "delayed hold abandoned after a late first signal, then a timer from the abandonment", first: 1200 * ms,
			waits: []time.Duration{1250 * ms}, kills: []time.Duration{2250 * ms}, ack: ackCut, refuses: "delay did not execute"},
		{name: "delayed hold abandoned within the call limit of a first signal at 600 ms", first: 600 * ms,
			kills: []time.Duration{1100 * ms}, ack: ackCut, refuses: "delay did not execute"},
		{name: "delayed hold abandoned just before the call limit", first: 600 * ms,
			kills: []time.Duration{600*ms + call - ms}, ack: ackCut, refuses: "delay did not execute"},
		// The call limit runs from the signal call's start, which the recorder
		// cannot see: the kill reaches it some time later, and the daemon check
		// before the call is the last request that precedes the start. So the bound
		// runs from the daemon check, not from the kill's arrival. Here the check
		// is at 600 ms and the kill at 650 ms. A next request at 1.62 s comes 1.02 s
		// after the check but 970 ms after the kill, and an honest cut leaves just
		// that: the product's next request follows the cut at once, while the
		// kill's arrival lags the start of the call. The second case sends a wait
		// there, which is refused as a wait whatever its time.
		{name: "delayed acknowledgement cut a call limit after the daemon check", first: 650 * ms, checkLead: 50 * ms,
			kills: []time.Duration{1620 * ms}, ack: ackCut},
		{name: "delayed hold cut a call limit after the daemon check", first: 650 * ms, checkLead: 50 * ms,
			waits: []time.Duration{1620 * ms}, kills: []time.Duration{1700 * ms}, ack: ackCut, refuses: "delay did not execute"},
		// And not from the inspection before the daemon check: the call starts after
		// the check, so a next request a call limit after the inspection but short
		// of one after the check is the product giving up early.
		{name: "delayed hold abandoned within a call limit of the daemon check", first: 650 * ms, checkLead: 50 * ms,
			inspectLead: 40 * ms, kills: []time.Duration{1590 * ms}, ack: ackCut, refuses: "delay did not execute"},
		{name: "acknowledgement replied early", first: first, kills: []time.Duration{grace + 100*ms}, ack: ackEarly, refuses: "delay did not execute"},
		{name: "cancelled before the engine accepted", first: first, kills: []time.Duration{grace + 100*ms}, ack: ackStalled, refuses: "delay did not execute"},
		{name: "no reply and not cancelled", first: first, kills: []time.Duration{grace + 100*ms}, ack: ackNone, refuses: "delay did not execute"},
	}
}

// A world holding only what lifecycleSignals reads: the identities, the row's
// first signal, delay and cancellation time, and the journal c describes, each
// kill carrying the fresh checks and 0600 record that lifecycleRevalidated wants.
func lifecycleWindowWorld(t *testing.T, signal string, delay time.Duration, c lifecycleWindowCase) *world {
	t.Helper()
	worker := strings.Repeat("a", 64)
	w := &world{
		doctor:   &doctorWorld{engineID: "engine", imageID: "sha256:" + strings.Repeat("b", 64)},
		approval: &approvalWorld{project: "project"},
		session:  &sessionWorld{token: "token"},
		recovery: &recoveryNative{worker: worker},
	}
	w.offline = &offlineRuntime{api: &offlineAPI{w: w, created: []string{worker}}}
	trace := filepath.Join(t.TempDir(), "signals.log")
	if err := os.WriteFile(trace, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cancelled := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	observed := map[string]any{"Id": worker, "Name": "/itos-container-token", "Image": w.doctor.imageID,
		"Config": map[string]any{"Labels": w.sessionLabels()}}
	record := sessionRecord{Bytes: fmt.Sprintf(`{"container_id":%q,"project":"project"}`, worker), Mode: 0600}
	var journal []offlineRequest
	send := func(signal string, after, checkLead, inspectLead time.Duration) int {
		at := cancelled.Add(after)
		// In the product's order: recoverySignal inspects, then checks the daemon,
		// and only then signals.
		checked := at.Add(-checkLead)
		journal = append(journal,
			offlineRequest{Method: "GET", Path: "/containers/" + worker + "/json", Status: 200, Observation: observed, At: checked.Add(-inspectLead)},
			offlineRequest{Method: "GET", Path: "/info", Status: 200, Observation: map[string]any{"ID": "engine"}, At: checked},
			offlineRequest{Method: "POST", Path: "/containers/" + worker + "/kill", Query: "signal=" + signal, Status: 204,
				Observation: observed, Record: record, At: at, Replied: at})
		return len(journal) - 1
	}
	first := &journal[send(signal, c.first, c.checkLead, c.inspectLead)]
	switch c.ack {
	case ackHeld:
		first.Replied = first.At.Add(delay)
	case ackCut:
		first.Cancelled, first.Replied = true, time.Time{}
	case ackStalled:
		first.Cancelled, first.Status, first.Replied = true, 502, time.Time{}
	case ackEarly:
		first.Replied = first.At.Add(100 * time.Millisecond)
	case ackNone:
		first.Replied = time.Time{}
	}
	for _, after := range c.waits {
		journal = append(journal, offlineRequest{Method: "POST", Path: "/containers/" + worker + "/wait", Status: 200,
			At: cancelled.Add(after)})
	}
	for _, after := range c.kills {
		next := signal
		if !c.repeat {
			next = "SIGKILL"
		}
		send(next, after, 0, 0)
	}
	w.lifecycle = &lifecycleNative{first: signal, signalled: cancelled, delay: delay, aliveBeforeGrace: true,
		journalAtReturn: journal, signalTrace: trace}
	return w
}
