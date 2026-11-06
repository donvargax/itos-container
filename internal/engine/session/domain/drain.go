package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"time"
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
		}
	}()
	return done
}
