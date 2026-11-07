// Package signals keeps host interrupt handling active throughout owned cleanup.
package signals

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/run/port"
)

// Runner supports callers already at the frozen handoff. The command assembly
// uses Capture before preflight and retains it through diagnostic delivery.
type Runner struct{ Next port.Runner }

func (r Runner) Run(ctx context.Context, frozen approval.Durable, argv []string, streams session.Streams) session.RunResult {
	interrupts, release := Capture(ctx)
	defer release()
	return r.Next.Run(interrupts, frozen, argv, streams)
}

// Capture records the first signal and consumes repeats until release. The
// caller releases only after the runner has finished its bounded teardown.
func Capture(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
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
