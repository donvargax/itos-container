package domain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestQueuedCheckoutOperationCannotResumeAfterInterruption(t *testing.T) {
	for _, cancelNow := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			gate := make(chan struct{}, 1) // another operation still owns the pin
			done := make(chan error, 1)
			called := false
			go func() {
				_, err := checkoutCall(ctx, gate, "checkout inspection", func() bool {
					called = true
					return true
				})
				done <- err
			}()
			synctest.Wait()
			start := time.Now()
			want := 10 * time.Second
			if cancelNow {
				cancel()
				want = 0
			}
			if err := <-done; err == nil || time.Since(start) != want {
				t.Error("waiting for the checkout pin lost its interruption bound", err, time.Since(start))
			}
			gate <- struct{}{}
			synctest.Wait()
			if called {
				t.Error("an abandoned queued operation touched the checkout")
			}
			// The late no-op must release access for eventual handle closure.
			select {
			case <-gate:
			default:
				t.Error("abandoned checkout operation retained the pin")
			}
		})
	}
}
