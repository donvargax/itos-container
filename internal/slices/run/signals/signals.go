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
	interrupts, release := Capture(ctx, streams.Terminal != nil)
	defer release()
	return r.Next.Run(interrupts, frozen, argv, streams)
}

// Capture records the first signal and consumes repeats until release. The
// caller releases only after the runner has finished its bounded teardown.
// With terminal set, a hangup signal is consumed for the whole run too: the
// terminal lease ends its own SIGHUP subscription once it has reported the
// disconnect, while owned teardown and diagnostic delivery are still running,
// so a later hangup signal must neither kill this process nor cancel it.
func Capture(parent context.Context, terminal bool) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	received := make(chan os.Signal, 2)
	closed := make(chan struct{})
	done := make(chan struct{})
	subscribed := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if terminal {
		subscribed = append(subscribed, syscall.SIGHUP)
	}
	signal.Notify(received, subscribed...)
	go func() {
		defer close(done)
		for {
			select {
			case s := <-received:
				if s == syscall.SIGHUP {
					// Consumed, never forwarded: passive hangup observation
					// belongs to the terminal lease alone.
					continue
				}
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
