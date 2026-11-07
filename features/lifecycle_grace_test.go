package features

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func TestLifecycleGraceObservationPreservesDelayedEvidence(t *testing.T) {
	for _, delay := range []time.Duration{0, 5 * time.Second, 11 * time.Second} {
		t.Run(delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				p := &recoveryProcess{done: make(chan struct{})}
				go func() { time.Sleep(10 * time.Second); close(p.done) }()
				var signals []string
				var times []time.Duration
				alive := lifecycleRepeatedSignals(p, start, func(signal string) {
					signals = append(signals, signal)
					times = append(times, time.Since(start))
				}, func() bool {
					// Observe the worker before its allowed escalation, but delay
					// the reply beyond it. The observation must remain valid.
					observed := time.Since(start) < 10*time.Second
					time.Sleep(delay)
					return observed
				})
				if !alive || !slices.Equal(signals, []string{"SIGINT", "SIGTERM"}) || !slices.Equal(times, []time.Duration{2 * time.Second, 8 * time.Second}) {
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
