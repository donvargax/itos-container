package command

import (
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
)

func acquireTerminal(startup context.Context, provider terminal.Provider) (terminal.Lease, error) {
	if provider == nil {
		return nil, &terminal.Failure{Kind: terminal.InvalidTerminal}
	}
	// Acquisition shares startup's budget, but the acquired observer must outlive
	// startup and host cancellation until the lifecycle explicitly restores it.
	lifetime, cancel := context.WithCancel(context.WithoutCancel(startup))
	type acquired struct {
		lease terminal.Lease
		err   error
	}
	started, pending := make(chan struct{}), make(chan acquired, 1)
	r, err := budget.Acquire(startup, "terminal acquisition", func(context.Context) acquired {
		close(started)
		l, e := provider.Acquire(lifetime)
		a := acquired{l, e}
		pending <- a
		return a
	}, func(acquired) {
		// A late lease is closed by acquireTerminal itself before it returns;
		// closing here as well would close one lease twice.
	})
	if err != nil {
		cancel()
		// Join a started acquisition so a late lease is closed and its raw
		// state rolled back before this process can return. The provider's
		// own work is finite; a refusal that never began has nothing to join.
		select {
		case <-started:
			a := <-pending
			if a.lease != nil {
				a.lease.Close()
			}
		default:
		}
		return nil, err
	}
	if r.err != nil {
		if r.lease != nil {
			r.lease.Close()
		}
		cancel()
		return nil, r.err
	}
	return terminalLease{r.lease, cancel}, nil
}

// terminalLease ends the acquisition lifetime after the leased observer ends.
type terminalLease struct {
	terminal.Lease
	cancel context.CancelFunc
}

func (l terminalLease) Close() terminal.Result { r := l.Lease.Close(); l.cancel(); return r }
