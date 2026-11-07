package domain

import (
	"context"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

// Checkout operations, including readers borrowing its pin, retain exclusive
// access until their actual return. Ending the controller wait cannot close
// mutable adapter state underneath an uninterruptible filesystem call.
func checkoutCall[T any](ctx context.Context, gate chan struct{}, name string, fn func() T) (T, error) {
	return budget.Call(ctx, name, func(call context.Context) T {
		// Call bounds the wait for the gate too. A queued operation may acquire
		// it after cancellation or Close, but must never touch the adapter then.
		<-gate
		defer func() { gate <- struct{}{} }()
		if budget.Cause(call) != nil {
			var zero T
			return zero
		}
		return fn()
	})
}

func checkoutCall2[A, B any](ctx context.Context, gate chan struct{}, name string, fn func() (A, B)) (A, B, error) {
	type result struct {
		a A
		b B
	}
	r, err := checkoutCall(ctx, gate, name, func() result { a, b := fn(); return result{a, b} })
	return r.a, r.b, err
}

func (b boundedCheckout) Close() {
	select {
	case <-b.gate:
		b.Checkout.Close()
		b.gate <- struct{}{}
	default:
		go func() { <-b.gate; b.Checkout.Close(); b.gate <- struct{}{} }()
	}
}
