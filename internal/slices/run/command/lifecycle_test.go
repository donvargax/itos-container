package command

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestLifecycleResultPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   session.RunResult
		code     int
		messages []string
	}{
		{"unknown interrupt", session.RunResult{Signal: "SIGINT", Started: true}, 130, []string{"run.cancelled SIGINT", "unknown"}},
		{"unstarted term", session.RunResult{Signal: "SIGTERM"}, 143, []string{"run.cancelled SIGTERM", "not-started"}},
		{"zero", session.RunResult{Signal: "SIGTERM", Exited: true}, 0, []string{"run.cancelled SIGTERM", "workload outcome 0"}},
		{"nonzero", session.RunResult{Signal: "SIGINT", Exited: true, Exit: 17, Stream: session.StreamTimeout}, 17, []string{"run.cancelled SIGINT", "workload outcome 17", "run.stream-timeout"}},
		{"stream failure", session.RunResult{Exited: true, Stream: session.StreamTimeout}, 75, []string{"run.stream-timeout"}},
		{"lost engine", session.RunResult{Result: session.Result{Outcome: session.EngineLost}}, 75, []string{"run.engine-lost", "unknown"}},
		{"teardown timeout", session.RunResult{Teardown: session.TeardownTimeout}, 75, []string{"run.teardown-timeout"}},
		{"lost engine then timeout", session.RunResult{Result: session.Result{Outcome: session.EngineLost}, Teardown: session.TeardownTimeout}, 75, []string{"run.engine-lost", "run.teardown-timeout"}},
		{"refusal then timeout", session.RunResult{Result: session.Result{Outcome: session.IsolationUnverified}, Teardown: session.TeardownTimeout}, 1, []string{"run.isolation-unverified", "run.teardown-timeout"}},
		{"interrupt then timeout", session.RunResult{Signal: "SIGTERM", Started: true, Teardown: session.TeardownTimeout}, 143, []string{"run.cancelled SIGTERM", "unknown", "run.teardown-timeout"}},
		{"exit then timeout", session.RunResult{Exited: true, Exit: 17, Teardown: session.TeardownTimeout}, 17, []string{"run.teardown-timeout"}},
		{"unknown with cleanup failure", session.RunResult{Signal: "SIGTERM", Started: true, Result: session.Result{Reference: "protected", Cleanup: session.Incomplete}}, 143, []string{"run.cancelled SIGTERM", "unknown", "run.cleanup-temporary"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output strings.Builder
			code := reportRun(&cli.UI{Stderr: &output, Classify: Classify}, tc.result)
			if code != tc.code {
				t.Fatal(code, output.String())
			}
			for _, message := range tc.messages {
				if !strings.Contains(output.String(), message) {
					t.Fatal("missing", message, output.String())
				}
			}
		})
	}
}

type heldDiagnostic struct {
	entered chan struct{}
	release <-chan struct{}
}

func (w heldDiagnostic) Write(data []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(data), nil
}

func TestUnreadDiagnosticUsesRemainingTeardown(t *testing.T) {
	for _, tc := range []struct {
		name          string
		elapsed, wait time.Duration
		result        session.RunResult
		code          int
	}{
		{"call limit", 11 * time.Second, 10 * time.Second, session.RunResult{Signal: "SIGTERM", Exited: true, Exit: 17}, 17},
		{"remaining teardown", 25 * time.Second, 5 * time.Second, session.RunResult{Signal: "SIGTERM", Exited: true, Exit: 17}, 17},
		{"unknown exit call limit", 11 * time.Second, 10 * time.Second, session.RunResult{Started: true, Result: session.Result{Cleanup: session.Complete}}, 75},
		{"unknown exit remaining teardown", 25 * time.Second, 5 * time.Second, session.RunResult{Started: true, Result: session.Result{Cleanup: session.Complete}}, 75},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				deadline, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				time.Sleep(tc.elapsed)
				until, _ := deadline.Deadline()
				result := tc.result
				result.Deadline = until
				release := make(chan struct{})
				entered := make(chan struct{})
				done := make(chan int, 1)
				go func() {
					done <- reportRun(&cli.UI{Stderr: heldDiagnostic{entered, release}, Classify: Classify}, result)
				}()
				<-entered
				start := time.Now()
				code := <-done
				elapsed := time.Since(start)
				close(release)
				synctest.Wait()
				if code != tc.code || elapsed != tc.wait {
					t.Fatal("diagnostic delivery exceeded its call or teardown limit", code, elapsed)
				}
			})
		})
	}
}
