package domain

import (
	"context"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (c finiteConnection) Wait(ctx context.Context, id string) (int, port.Outcome) {
	type result struct {
		exit    int
		failure port.Outcome
	}
	done := make(chan result, 1)
	go func() {
		<-c.gate
		defer func() { c.gate <- struct{}{} }()
		if budget.Cause(ctx) != nil {
			return
		}
		exit, failure := c.RunConnection.Wait(ctx, id)
		done <- result{exit, failure}
	}()
	// The caller supplies cancellation, grace or teardown's deadline. A healthy
	// foreground subscription has no finite-call or startup lifetime. Retain the
	// connection until even a non-cancellable identity syscall has finished, but
	// discard its exit observation once the controller has stopped waiting.
	select {
	case r := <-done:
		if budget.Cause(ctx) == nil {
			return r.exit, r.failure
		}
	case <-ctx.Done():
	}
	return 0, port.ExecutionFailed
}
