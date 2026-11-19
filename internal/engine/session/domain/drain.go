package domain

import (
	"context"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
)

func startDrain(ctx context.Context, a port.Attachment, lease terminal.Lease, runOutcome port.Outcome) <-chan port.Outcome {
	done := make(chan port.Outcome, 1)
	drain, cancel := context.WithTimeout(ctx, budget.CallDuration)
	go func() {
		defer cancel()
		var failure port.Outcome
		defer func() {
			// Keep output processing disabled through the last attachment byte.
			// Restoration follows the bounded drain independently of cleanup;
			// Runner retains its error through the lease's idempotent Close.
			if lease != nil {
				lease.Close()
			}
			done <- failure
		}()
		if a == nil {
			return
		}
		failure = a.Drain(drain)
		if drain.Err() != nil {
			failure = port.StreamTimeout
		}
		if failure != "" {
			// Release the owned engine stream now. Keeping it open until stop
			// returns can hold the daemon's exit response behind output that
			// the caller will never read. Close never waits on caller I/O.
			a.Close()
			// A lost terminal can fail a pending read or write. Drain retains
			// that forwarding error even after the run has classified its cause.
			if failure == port.ExecutionFailed && (runOutcome == port.TerminalDisconnected || runOutcome == port.TerminalUnavailable) {
				failure = ""
			}
		}
	}()
	return done
}
