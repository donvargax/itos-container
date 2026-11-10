package domain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

func TestConnectionCallCannotResumeAfterQueuedCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, end := budget.Start(context.Background(), time.Second, "startup")
		defer end()
		// Another operation owns the connection until after this wait expires.
		gate := make(chan struct{}, 1)
		calls := 0
		_, err := connectionCall(ctx, gate, "start acknowledgement", func(context.Context) bool {
			calls++
			return true
		})
		if err == nil {
			t.Fatal("queued call exceeded its startup deadline")
		}
		gate <- struct{}{}
		synctest.Wait()
		if calls != 0 {
			t.Fatal("expired queued operation accessed the connection")
		}
		// Returning the gate must not revive a second operation on the same scope.
		_, err = connectionCall(ctx, gate, "start acknowledgement", func(context.Context) bool {
			calls++
			return true
		})
		if err == nil || calls != 0 {
			t.Fatal("expired scope authorized another operation")
		}
	})
}
