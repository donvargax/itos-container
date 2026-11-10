package domain

import (
	"context"
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

func TestEgressWatchPreservesTheWinningWaitResult(t *testing.T) {
	for _, tc := range []struct {
		name          string
		workerFailure port.Outcome
		proxyFailure  bool
		proxyFirst    bool
		wantExit      int
		wantExited    bool
		wantFailure   port.Outcome
	}{
		{"worker failure", port.EngineLost, false, false, 23, false, port.EngineLost},
		{"proxy exit", "", false, true, 0, false, port.ProxyExited},
		{"proxy wait failure", "", true, true, 0, false, port.ExecutionFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runner, registry, engine, id, r := seedEgressSession(t, 8)
				i, c := openEgressTestSession(t, registry, engine, id)
				a, failure := c.Attach(t.Context(), r.ContainerID, port.Streams{})
				if failure != "" {
					t.Fatal(failure)
				}
				defer a.Close()
				engine.Exit, engine.WaitFailure = 23, tc.workerFailure
				workerExit, proxyExit := make(chan struct{}), make(chan struct{})
				engine.WaitRelease, engine.ProxyExit = workerExit, proxyExit
				if tc.proxyFailure {
					engine.EgressFault = "proxy wait"
				}
				type result struct {
					exit    int
					exited  bool
					failure port.Outcome
				}
				done := make(chan result, 1)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				go func() {
					exit, exited, failure := runner.waitEgress(ctx, c, a, i.Binding(), r)
					done <- result{exit, exited, failure}
				}()
				synctest.Wait()
				if tc.proxyFirst {
					close(proxyExit)
				} else {
					close(workerExit)
				}
				got := <-done
				if got.exit != tc.wantExit || got.exited != tc.wantExited || got.failure != tc.wantFailure {
					t.Fatalf("winning wait = %#v, want %d/%v/%s", got, tc.wantExit, tc.wantExited, tc.wantFailure)
				}
				synctest.Wait()
				if !slices.Contains(engine.Events, "wait") || !slices.Contains(engine.Events, "proxy wait") {
					t.Fatal("both subscriptions must terminate", engine.Events)
				}
			})
		})
	}
}
