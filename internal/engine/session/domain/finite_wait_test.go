package domain

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestFiniteWaitCannotResumeAfterQueuedCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, _, engine := st.Fixture()
		raw := &st.RunEngine{Engine: engine, Exit: 17}
		connection, failure := raw.OpenRun(context.Background(), b)
		if failure != "" {
			t.Fatal(failure)
		}
		// A prior abandoned operation still holds the connection. Cancellation
		// must end this queued wait and forbid entering the adapter afterward.
		gate := make(chan struct{}, 1)
		c := finiteConnection{connection, gate}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		exit, failure := c.Wait(ctx, engine.CreateID)
		if exit != 0 || failure != port.ExecutionFailed {
			t.Fatal("cancelled wait returned a workload exit", exit, failure)
		}
		gate <- struct{}{}
		synctest.Wait()
		c.Close()
		if slices.Contains(raw.Events, "wait") {
			t.Fatal("cancelled queued wait accessed the connection", raw.Events)
		}
	})
}

func TestFiniteWaitRejectsExitReturnedAfterCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, _, engine := st.Fixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// The fake completes the engine observation, then cancellation becomes
		// visible before its response reaches the controller.
		raw := &st.RunEngine{Engine: engine, Exit: 17, AfterWait: cancel}
		connection, failure := (finiteEngine{raw}).OpenRun(context.Background(), b)
		if failure != "" {
			t.Fatal(failure)
		}
		defer connection.Close()
		exit, failure := connection.Wait(ctx, engine.CreateID)
		if exit != 0 || failure != port.ExecutionFailed {
			t.Fatal("late wait response became an authoritative exit", exit, failure)
		}
	})
}
