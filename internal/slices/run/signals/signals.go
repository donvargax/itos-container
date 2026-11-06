// Package signals keeps host interrupt handling active throughout owned cleanup.
package signals

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	session "github.com/donvargax/itos-container/internal/engine/session/port"
)

// Capture records the first signal and consumes repeats until release. The
// caller releases only after the runner has finished its bounded teardown.
func Capture() (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(context.Background())
	received := make(chan os.Signal, 2)
	closed := make(chan struct{})
	done := make(chan struct{})
	signal.Notify(received, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer close(done)
		for {
			select {
			case s := <-received:
				name := "SIGINT"
				if s == syscall.SIGTERM {
					name = "SIGTERM"
				}
				cancel(session.Interrupt{Signal: name})
			case <-closed:
				return
			}
		}
	}()
	return ctx, func() {
		signal.Stop(received)
		close(closed)
		<-done
		cancel(nil)
	}
}
