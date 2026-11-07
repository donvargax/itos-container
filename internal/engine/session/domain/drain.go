package domain

import (
	"context"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func startDrain(ctx context.Context, a port.Attachment) <-chan port.Outcome {
	done := make(chan port.Outcome, 1)
	if a == nil {
		done <- ""
		return done
	}
	drain, cancel := context.WithTimeout(ctx, 10*time.Second)
	go func() {
		defer cancel()
		var failure port.Outcome
		defer func() { done <- failure }()
		failure = a.Drain(drain)
		if drain.Err() != nil {
			failure = port.StreamTimeout
			// Release the owned engine stream now. Keeping it open until stop
			// returns can hold the daemon's exit response behind output that
			// the caller will never read. Close never waits on caller I/O.
			a.Close()
		}
	}()
	return done
}
