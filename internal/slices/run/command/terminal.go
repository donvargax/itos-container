package command

import (
	"context"
	"errors"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
)

func acquireTerminal(startup context.Context, provider terminal.Provider) (terminal.Lease, error) {
	if provider == nil {
		return nil, &terminal.Failure{Kind: terminal.InvalidTerminal}
	}
	if err := budget.Cause(startup); err != nil {
		return nil, err
	}
	// The acquired observer must outlive startup until the lifecycle restores it.
	lifetime, cancel := context.WithCancel(context.WithoutCancel(startup))
	var lease terminal.Lease
	var acquireErr error
	done := make(chan struct{})
	go func() {
		lease, acquireErr = provider.Acquire(lifetime)
		close(done)
	}()
	_, cause := budget.Call(startup, "terminal acquisition", func(context.Context) struct{} {
		<-done
		return struct{}{}
	})
	if cause != nil {
		cancel()
		// Join even if the provider is still rolling back a partial acquisition.
		<-done
	} else if acquireErr == nil {
		return terminalLease{lease, cancel}, nil
	} else {
		cause = acquireErr
		cancel()
	}

	closeFailed := lease != nil && lease.Close().RestoreErr != nil
	if !terminalRollbackFailed(cause) && (closeFailed || terminalRollbackFailed(acquireErr)) {
		cause = errors.Join(cause, runFailure{outcome: session.TerminalRestore})
	}
	return nil, cause
}

// The provider can report rollback itself, or the command can discover it
// while closing a provisional lease after the acquisition budget ended.
func terminalRollbackFailed(err error) bool {
	var failure *terminal.Failure
	return errors.Is(err, runFailure{outcome: session.TerminalRestore}) ||
		(errors.As(err, &failure) && failure.RestoreErr != nil)
}

// terminalLease ends the acquisition lifetime after the leased observer ends.
type terminalLease struct {
	terminal.Lease
	cancel context.CancelFunc
}

func (l terminalLease) Close() terminal.Result { r := l.Lease.Close(); l.cancel(); return r }
