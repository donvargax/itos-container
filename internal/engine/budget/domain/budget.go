// Package domain bounds controller phases and finite operations without imposing
// a lifetime on acknowledged workload subscriptions.
package domain

import (
	"context"
	"time"

	"github.com/donvargax/itos-container/internal/engine/budget/port"
)

// Failure carries only controller-selected names, never external error payloads.
type Failure = port.Failure

type scopeKey struct{}
type startupKey struct{}
type scope struct {
	phase string
	abort context.CancelCauseFunc
}

func Start(parent context.Context, duration time.Duration, phase string) (context.Context, context.CancelFunc) {
	deadline, end := context.WithTimeoutCause(parent, duration, Failure{Phase: phase})
	ctx, abort := context.WithCancelCause(deadline)
	return context.WithValue(ctx, scopeKey{}, scope{phase, abort}), func() { abort(nil); end() }
}

// Cause also checks monotonic time: at an exact boundary the timer goroutine
// need not have published cancellation yet. A prior cause still wins.
func Cause(ctx context.Context) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if expired(ctx) {
		if s, ok := ctx.Value(scopeKey{}).(scope); ok {
			return Failure{Phase: s.phase}
		}
		return context.DeadlineExceeded
	}
	return nil
}
func expired(ctx context.Context) bool { d, ok := ctx.Deadline(); return ok && !time.Now().Before(d) }

// Lifetime is the phase context retained by an acquired handle. Its establishment
// call's shorter deadline must not poison subsequent independently bounded calls.
func Lifetime(ctx context.Context) context.Context { return port.Lifetime(ctx) }

func WithStartup(ctx, startup context.Context) context.Context {
	return context.WithValue(ctx, startupKey{}, startup)
}
func Startup(ctx context.Context) (context.Context, context.CancelFunc) {
	if startup, ok := ctx.Value(startupKey{}).(context.Context); ok {
		return startup, func() {}
	}
	return Start(ctx, StartupDuration, "startup")
}

type pair[A, B any] struct {
	a A
	b B
}

// Call ends the controller wait even when a filesystem syscall cannot be
// interrupted. A late result cannot resume the controller or overwrite its cause.
// No second mutation is issued. Acquired handles use Acquire to release late ones.
func Call[T any](ctx context.Context, operation string, fn func(context.Context) T) (T, error) {
	return Acquire(ctx, operation, fn, func(T) {})
}
func Call2[A, B any](ctx context.Context, operation string, fn func(context.Context) (A, B)) (A, B, error) {
	r, err := Call(ctx, operation, func(call context.Context) pair[A, B] { a, b := fn(call); return pair[A, B]{a, b} })
	return r.a, r.b, err
}
func Acquire[T any](ctx context.Context, operation string, fn func(context.Context) T, discard func(T)) (T, error) {
	var zero T
	if cause := Cause(ctx); cause != nil {
		return zero, cause
	}
	call, end := context.WithTimeout(ctx, CallDuration)
	defer end()
	call = port.WithLifetime(call, ctx)
	done := make(chan T)
	go func() {
		r := fn(call)
		select {
		case done <- r:
		case <-call.Done():
			discard(r)
		}
	}()
	select {
	case r := <-done:
		if call.Err() == nil && !expired(call) {
			return r, nil
		}
		discard(r)
	case <-call.Done():
	}
	if cause := Cause(ctx); cause != nil {
		return zero, cause
	}
	failure := Failure{Operation: operation}
	if s, ok := ctx.Value(scopeKey{}).(scope); ok {
		s.abort(failure)
	}
	return zero, failure
}
