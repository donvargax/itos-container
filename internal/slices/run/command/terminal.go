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
	r, err := budget.Acquire(startup, "terminal acquisition", func(context.Context) acquired {
		l, e := provider.Acquire(lifetime)
		return acquired{l, e}
	}, func(r acquired) {
		if r.lease != nil {
			r.lease.Close()
		}
		cancel()
	})
	if err != nil {
		cancel()
		return nil, err
	}
	if r.err != nil {
		cancel()
		return nil, r.err
	}
	return terminalLease{r.lease, cancel}, nil
}

type terminalLease struct {
	terminal.Lease
	cancel context.CancelFunc
}

func (l terminalLease) Close() terminal.Result { r := l.Lease.Close(); l.cancel(); return r }
