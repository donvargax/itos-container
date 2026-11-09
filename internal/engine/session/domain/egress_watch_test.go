package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"testing"
	"time"
)

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
