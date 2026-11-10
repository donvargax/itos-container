package domain

import (
	"slices"
	"testing"
	"testing/synctest"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestEgressWorkerExitReleasesWatcher(t *testing.T) {
	// The bubble must finish every goroutine, including the proxy watch whose
	// cancellation result is no longer consumed after the worker wins.
	synctest.Test(t, func(t *testing.T) {
		runner, registry, engine, _, r := seedEgressSession(t, 8)
		b := registry.Store.Codec.(*f.Codec).Value
		c, failure := engine.OpenRun(t.Context(), b)
		if failure != "" {
			t.Fatal(failure)
		}
		defer c.Close()
		a, failure := c.Attach(t.Context(), r.ContainerID, port.Streams{})
		if failure != "" {
			t.Fatal(failure)
		}
		defer a.Close()

		workerExit := make(chan struct{})
		engine.WaitRelease = workerExit
		engine.Exit = 17
		// No proxy exit: its wait can finish only when the controller cancels it.
		engine.ProxyExit = nil
		type result struct {
			exit    int
			exited  bool
			failure port.Outcome
		}
		done := make(chan result, 1)
		go func() {
			exit, exited, failure := runner.waitEgress(t.Context(), c, a, b, r)
			done <- result{exit, exited, failure}
		}()
		synctest.Wait()
		if !slices.Contains(engine.Events, "wait") || !slices.Contains(engine.Events, "proxy wait") {
			t.Fatal("worker and proxy waits did not both start", engine.Events)
		}
		select {
		case got := <-done:
			t.Fatalf("wait returned before the worker exited: %#v", got)
		default:
		}

		close(workerExit)
		got := <-done
		if got.exit != 17 || !got.exited || got.failure != "" {
			t.Errorf("proxy cancellation replaced the worker result: %#v", got)
		}
		synctest.Wait()
	})
}
