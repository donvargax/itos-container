package domain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestStartupKeepsClassifiedRefusal(t *testing.T) {
	for _, failure := range []port.Outcome{port.Refused, port.IsolationUnverified} {
		ctx, cancel := context.WithCancelCause(context.Background())
		got := startupOperation(ctx, "verify", func(context.Context) port.Outcome { cancel(port.Interrupt{Signal: "SIGINT"}); return failure })
		if got != failure {
			t.Fatal("signal replaced refusal", got)
		}
	}
}
func TestStartupOperationBoundaries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, duration := range []time.Duration{5 * time.Second, 10 * time.Second, 120 * time.Second} {
			ctx, end := budget.Start(context.Background(), duration, "startup")
			state := &startupState{}
			ctx = context.WithValue(ctx, startupKey{}, state)
			f := startupOperation(ctx, "create response", func(call context.Context) port.Outcome { <-call.Done(); return port.Uncertain })
			want := port.CallTimeout
			if duration <= 10*time.Second {
				want = port.StartupTimeout
			}
			if f != want || state.failure != want || state.operation != "create response" {
				t.Fatal(f, state)
			}
			end()
		}
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(port.Interrupt{Signal: "SIGTERM"})
		if startupOperation(ctx, "start", func(context.Context) port.Outcome { t.Fatal("entered after cancellation"); return "" }) != port.ExecutionFailed {
			t.Fatal("cancellation cause")
		}
		r := startupResult(ctx, ctx, context.Cause(ctx), "intent")
		if r.Signal != "SIGTERM" || r.Started || r.Outcome != "" {
			t.Fatal(r)
		}
	})
}
