package main

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

// Freeze diagnostics when the command returns: main calls os.Exit immediately.
// A later harness wait must never supply bytes for this observation.
type lifecycleDiagnostic struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	once    sync.Once
	delay   time.Duration
	release <-chan struct{}
}

func (w *lifecycleDiagnostic) Write(data []byte) (int, error) {
	w.once.Do(func() {
		if w.release != nil {
			<-w.release
		}
		time.Sleep(w.delay)
	})
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(data)
}

func (w *lifecycleDiagnostic) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func TestLifecycleTeardownReview(t *testing.T) {
	selection := os.Getenv("SLICE_12_REVIEW")
	if selection == "" {
		t.Skip("selected by lifecycle scenarios")
	}
	parts := strings.Split(selection, ":")
	if len(parts) != 3 {
		t.Fatal("invalid review case", selection)
	}
	for _, backend := range []string{"ports", "protected records"} {
		t.Run(backend, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				if parts[0] == "output" {
					lifecycleDiagnosticBudget(t, backend, parts)
					return
				}
				lifecycleTeardownReview(t, backend, parts)
			})
		})
	}
}

func lifecycleTeardownReview(t *testing.T, backend string, parts []string) {
	// Same-path healthy control observes and removes a worker normally.
	control := lifecycleLocalFixture(t, backend)
	control.m.e.Exit = 17
	code, diagnostic, _ := control.invoke(context.Background())
	if code != 17 || diagnostic != "" || len(control.m.e.Resources) != 1 || control.count("start") != 1 {
		t.Fatal("healthy control failed", code, diagnostic)
	}
	l := lifecycleLocalFixture(t, backend)
	want, known := 75, false
	if parts[0] == "diagnostics" {
		child, err := strconv.Atoi(parts[1])
		if err != nil {
			t.Fatal(err)
		}
		l.m.e.Exit, known = child, true
		if child != 0 {
			want = child
		}
	} else {
		l.m.e.WaitFailure = sp.Outcome(parts[1])
		l.m.e.Exit = 17
		if parts[2] == "removal" {
			want, known = 17, true
		}
	}
	var began time.Time
	opens := 0
	l.hook = func(call context.Context, event string) {
		if event == "wait" && began.IsZero() {
			began = time.Now()
		}
		if began.IsZero() {
			return
		}
		if event == "session-open" {
			opens++
		}
		if event == "open" {
			// The first failed wait does not invent an exit. A fresh successful
			// observation in the removal case can later establish exit 17.
			l.m.e.WaitFailure = ""
		}
		if parts[0] == "loss" && parts[2] == "removal" && opens < 2 {
			return
		}
		if parts[0] == "diagnostics" && event != "check" && event != "inspect" {
			return
		}
		switch event {
		case "session-open", "open", "check", "inspect":
			select {
			case <-time.After(9 * time.Second):
			case <-call.Done():
			}
		}
	}
	w := &lifecycleDiagnostic{}
	release := make(chan struct{})
	if parts[2] == "delayed" {
		w.delay = 100 * time.Millisecond
	}
	if parts[2] == "blocked" {
		w.release = release
	}
	var out bytes.Buffer
	code = l.invokeWithStreams(context.Background(), &out, w)
	elapsed, atReturn := time.Since(began), w.String()
	close(release)
	// Only dispose of abandoned test writers after freezing the observation.
	time.Sleep(w.delay)
	synctest.Wait()
	if began.IsZero() || opens == 0 || elapsed > 30*time.Second || code != want {
		t.Errorf("teardown result: exit %d, want %d; elapsed %s; %s", code, want, elapsed, atReturn)
	}
	id := strings.Repeat("a", 64)
	if parts[2] != "blocked" {
		for _, message := range []string{"run.teardown-timeout", "itos container stop " + id} {
			if !strings.Contains(atReturn, message) {
				t.Errorf("diagnostic missing before command return: %s; got %q", message, atReturn)
			}
		}
	} else if elapsed != 30*time.Second {
		t.Error("blocked diagnostics did not use the original teardown bound", elapsed)
	}
	if parts[0] == "loss" && !strings.Contains(w.String(), "run."+parts[1]) {
		t.Errorf("teardown erased original failure run.%s: %s", parts[1], w.String())
	}
	if strings.Contains(w.String(), "synthetic-sensitive-argv") {
		t.Error("diagnostics leaked workload argv")
	}
	l.hook = nil
	entry, failure := l.registry.Open(context.Background(), id, true)
	if failure != "" {
		t.Fatal("lost retained session", failure)
	}
	record := entry.Record()
	if record.State == "stopped" || entry.Intent().Record().Stage == "absent" || (record.WorkloadExit != nil) != known {
		t.Error("timeout fabricated completion or lost workload observation", record)
	}
	if known && record.WorkloadExit != nil && *record.WorkloadExit != l.m.e.Exit {
		t.Error("timeout changed observed exit", record)
	}
	entry.Close()
	// New recovery receives its own budget, with the original identity only.
	result := (domain.Recovery{Registry: l.registry, Engine: lifecycleEngine{l}}).Stop(context.Background(), id)
	if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || len(l.m.e.Resources) != 1 || l.count("create") != 1 || l.count("start") != 1 {
		t.Error("fresh recovery lost ownership or restarted work", result)
	}
}
