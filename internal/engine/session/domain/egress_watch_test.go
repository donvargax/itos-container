package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"slices"
	"testing"
	"time"
)

func TestEgressWatcherOpenInterrupt(t *testing.T) {
	for _, signal := range []string{"SIGINT", "SIGTERM"} {
		t.Run(signal, func(t *testing.T) {
			runner, durable, registry, engine, _ := freshEgressRunner()
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			opens := 0
			engine.After = func(event string) {
				if event == "open" {
					opens++
					if opens == 2 { // The worker launched; its independent proxy watch is opening.
						cancel(port.Interrupt{Signal: signal})
					}
				}
			}
			result := runner.Run(ctx, durable, []string{"probe"}, port.Streams{})
			if result.Signal != signal || result.Outcome != "" || !result.Started || !result.Exited || result.Exit != 17 || result.Cleanup != port.Complete {
				t.Errorf("watch-open interrupt lost signal or exit: %#v", result)
			}
			saved, failure := registry.Store.Codec.Decode(registry.Store.Records[result.Reference])
			if failure != "" || !saved.Completed() || !slices.Contains(engine.Events, signal) || len(engine.Resources) != 0 || len(engine.Networks) != 0 || len(engine.Anchors) != 0 {
				t.Errorf("interrupted worker was not terminated and released: %#v; %v", saved, engine.Events)
			}
		})
	}
}

func TestEgressInterruptPreservesLaunchRefusal(t *testing.T) {
	runner, durable, registry, engine, _ := freshEgressRunner()
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	engine.EgressFault = "proxy verify"
	refused := false
	engine.After = func(event string) {
		if event == "proxy verify" {
			refused = true
		}
	}
	// Deliver the signal after the refusal was observed, as launch releases its lock.
	registry.AfterClose = func(string) {
		if refused {
			cancel(port.Interrupt{Signal: "SIGINT"})
		}
	}
	result := runner.Run(ctx, durable, []string{"probe"}, port.Streams{})
	if context.Cause(ctx) != (port.Interrupt{Signal: "SIGINT"}) || result.Signal != "" || result.Outcome != port.IsolationUnverified || result.Cleanup != port.Complete || result.Started {
		t.Errorf("interrupt replaced isolation refusal: %#v", result)
	}
}

func TestEgressProxyExitDuringRun(t *testing.T) {
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
	exited := make(chan struct{})
	close(exited)
	engine.ProxyExit = exited
	engine.WaitUntilCancel = true
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, _, failure = runner.waitEgress(ctx, c, a, b, r)
	if failure != port.ProxyExited {
		t.Error("proxy exit not observed", failure)
	}
}
