// Package port carries the phase lifetime across finite handle acquisition.
package port

import "context"

type lifetimeKey struct{}

func WithLifetime(call, phase context.Context) context.Context {
	return context.WithValue(call, lifetimeKey{}, Lifetime(phase))
}
func Lifetime(ctx context.Context) context.Context {
	if parent, ok := ctx.Value(lifetimeKey{}).(context.Context); ok {
		return parent
	}
	return ctx
}

// Failure carries only controller-selected names, never external payloads.
type Failure struct{ Phase, Operation string }

func (Failure) Error() string { return "controller deadline exhausted" }
