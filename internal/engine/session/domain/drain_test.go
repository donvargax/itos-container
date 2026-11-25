package domain

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	tf "github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
)

func TestRegisteredRunReconcilesRetainedTerminalStreamFailure(t *testing.T) {
	for _, tc := range []struct {
		name            string
		outcome, stream port.Outcome
	}{
		{"hangup", port.TerminalDisconnected, ""},
		{"observation failed", port.TerminalUnavailable, ""},
		{"healthy terminal", port.ExecutionFailed, port.ExecutionFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runner, frozen, _, engine, _ := registeredLifecycleFixture()
				provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}, ObservationDelay: time.Hour}
				lease, err := provider.Acquire(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
				failures := make(chan port.Outcome, 1)
				engine.WaitUntilCancel = true
				engine.StdinFailures = failures
				engine.DrainFailure = port.ExecutionFailed
				done := make(chan port.RunResult, 1)
				go func() {
					done <- runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{Terminal: lease})
				}()
				synctest.Wait()
				switch tc.outcome {
				case port.TerminalDisconnected:
					provider.Hangup(false)
				case port.TerminalUnavailable:
					provider.Break()
				}
				failures <- port.ExecutionFailed
				result := <-done
				if result.Outcome != tc.outcome || result.Stream != tc.stream || result.Cleanup != port.Complete {
					t.Fatalf("outcome=%q stream=%q cleanup=%q", result.Outcome, result.Stream, result.Cleanup)
				}
			})
		})
	}
}
