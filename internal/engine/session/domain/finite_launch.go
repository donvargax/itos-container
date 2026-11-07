package domain

import (
	"context"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Keep connection state alive until the actual operation returns, even when a
// socket identity syscall ignores cancellation. Only the controller wait ends;
// a late result cannot re-enter launch, and a late attachment must be closed.
func connectionAcquire[T any](ctx context.Context, gate chan struct{}, name string, fn func(context.Context) T, discard func(T)) (T, error) {
	return budget.Acquire(ctx, name, func(call context.Context) T {
		<-gate
		defer func() { gate <- struct{}{} }()
		// A queued operation can acquire the gate after cancellation or Close.
		// It must not access the adapter or retry a mutation in that case.
		if budget.Cause(call) != nil {
			var zero T
			return zero
		}
		return fn(call)
	}, discard)
}

func connectionCall[T any](ctx context.Context, gate chan struct{}, name string, fn func(context.Context) T) (T, error) {
	return connectionAcquire(ctx, gate, name, fn, func(T) {})
}

func (c finiteConnection) Close() {
	select {
	case <-c.gate:
		c.RunConnection.Close()
		c.gate <- struct{}{}
	default:
		go func() { <-c.gate; c.RunConnection.Close(); c.gate <- struct{}{} }()
	}
}

func (c finiteConnection) CreateRun(ctx context.Context, r port.Record, argv []string) (string, port.Outcome) {
	type result struct {
		id string
		f  port.Outcome
	}
	v, err := connectionCall(ctx, c.gate, "create response", func(call context.Context) result {
		id, f := c.RunConnection.CreateRun(call, r, argv)
		return result{id, f}
	})
	if err != nil {
		return "", port.Uncertain
	}
	return v.id, v.f
}

func (c finiteConnection) VerifyRun(ctx context.Context, id string, argv []string) port.Outcome {
	f, err := connectionCall(ctx, c.gate, "boundary inspection", func(call context.Context) port.Outcome {
		return c.RunConnection.VerifyRun(call, id, argv)
	})
	if err != nil {
		return port.IsolationInspection
	}
	return f
}

func (c finiteConnection) Attach(ctx context.Context, id string, streams port.Streams) (port.Attachment, port.Outcome) {
	type result struct {
		a port.Attachment
		f port.Outcome
	}
	v, err := connectionAcquire(ctx, c.gate, "attach establishment", func(call context.Context) result {
		a, f := c.RunConnection.Attach(call, id, streams)
		return result{a, f}
	}, func(v result) {
		if v.a != nil {
			v.a.Close()
		}
	})
	if err != nil {
		return nil, port.ExecutionFailed
	}
	return v.a, v.f
}

func (c finiteConnection) Start(ctx context.Context, id string) port.Outcome {
	f, err := connectionCall(ctx, c.gate, "start acknowledgement", func(call context.Context) port.Outcome {
		return c.RunConnection.Start(call, id)
	})
	if err != nil {
		return port.ExecutionFailed
	}
	return f
}
