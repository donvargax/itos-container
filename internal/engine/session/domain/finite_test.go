package domain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type lateWorkspace struct {
	porttest.Workspace
	closes int
	closed chan time.Time
}

func (w *lateWorkspace) Pin(b port.Binding) port.Outcome {
	time.Sleep(11 * time.Second)
	return w.Workspace.Pin(b)
}
func (w *lateWorkspace) Close() { w.closes++; w.Workspace.Close(); w.closed <- time.Now() }

func TestFiniteWorkspaceRetainsLatePinUntilRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, end := budget.Start(context.Background(), 120*time.Second, "startup")
		defer end()
		gate := make(chan struct{}, 1)
		gate <- struct{}{}
		workspace := &lateWorkspace{closed: make(chan time.Time, 2)}
		w := finiteWorkspace{workspace, ctx, gate}
		start := time.Now()
		if f := w.Pin(port.Binding{}); f != port.Incomplete || time.Since(start) != 10*time.Second {
			t.Fatal(f, time.Since(start))
		}
		select {
		case <-workspace.closed:
			t.Fatal("pending pin closed before release")
		default:
		}
		if w.Validate() != port.Incomplete {
			t.Fatal("pending pin closed or reused before release")
		}
		closedAt := <-workspace.closed
		if closedAt.Sub(start) != 11*time.Second {
			t.Fatal("late close timing", closedAt.Sub(start))
		}
		synctest.Wait()
		if workspace.closes != 1 || !workspace.Closed {
			t.Fatal("late workspace handle leaked", workspace.closes)
		}
	})
}
func TestHeldOperationExpiresWhileAnotherCallOwnsHandle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, end := budget.Start(context.Background(), time.Second, "startup")
		defer end()
		gate := make(chan struct{}, 1)
		_, err := held(ctx, gate, "publication", func() bool { t.Fatal("used busy handle"); return true })
		if err == nil {
			t.Fatal("busy handle exceeded phase")
		}
	})
}
