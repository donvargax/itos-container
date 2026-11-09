package domain

import (
	"context"
	"errors"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type startupKey struct{}
type startupState struct {
	mountDiagnostic port.MountDiagnostic
	detectorExit    *int
	operation       string
	started         bool
	failure         port.Outcome
}

// Finite launch calls share the original phase deadline. Cancellation is checked
// between successful operations too: a returned handle never authorizes the next
// phase after a signal. fn uses finiteConnection's independently bounded calls;
// those retain pending adapter state and dispose of late acquired handles. Keep
// result assignment here on the controller goroutine. The state records only
// safe operation names.
func startupOperation(ctx context.Context, name string, fn func(context.Context) port.Outcome) port.Outcome {
	if budget.Cause(ctx) != nil {
		return startupFailure(ctx)
	}
	call, end := context.WithTimeout(ctx, budget.CallDuration)
	defer end()
	failure := fn(call)
	if budget.Cause(call) != nil && failure != port.Refused && failure != port.IsolationUnverified {
		failure = startupFailure(ctx)
		if budget.Cause(ctx) == nil {
			failure = port.CallTimeout
		}
		if state, ok := ctx.Value(startupKey{}).(*startupState); ok {
			state.operation, state.failure = name, failure
		}
	}
	return failure
}

func startupFailure(ctx context.Context) port.Outcome {
	var failure budget.Failure
	if errors.As(budget.Cause(ctx), &failure) {
		if failure.Phase != "" {
			return port.StartupTimeout
		}
		return port.CallTimeout
	}
	if errors.Is(budget.Cause(ctx), context.DeadlineExceeded) {
		return port.StartupTimeout
	}
	return port.ExecutionFailed
}

func startupResult(ctx, startup context.Context, err error, operation string) port.RunResult {
	r := port.RunResult{Outcome: startupFailure(startup), Operation: operation, Deadline: time.Now().Add(30 * time.Second)}
	if interrupted, ok := context.Cause(ctx).(port.Interrupt); ok {
		r.Signal, r.Outcome = interrupted.Signal, ""
		return r
	}
	var timed budget.Failure
	if errors.As(err, &timed) {
		if timed.Phase != "" {
			r.Outcome = port.StartupTimeout
		} else {
			r.Outcome = port.CallTimeout
		}
		if timed.Operation != "" {
			r.Operation = timed.Operation
		}
	}
	return r
}
