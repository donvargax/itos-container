package domain

import (
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"testing"
	"testing/synctest"
	"time"
)

func TestEgressSilentReadinessIsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, d, _, engine, _ := freshEgressRunner()
		engine.EgressFault = "proxy silent"
		before := time.Now()
		result := runner.Run(t.Context(), d, []string{"probe"}, port.Streams{})
		if result.Outcome != port.CallTimeout || result.Operation != "proxy readiness" || result.Started || time.Since(before) != budget.CallDuration || result.Cleanup != port.Complete {
			t.Errorf("silent proxy exceeded call or started worker: %#v duration %s", result, time.Since(before))
		}
	})
}
func TestEgressCopyTimeoutRetainsProxy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, registry, engine, id, r := seedEgressSession(t, 9)
		engine.EgressFault = "proxy copy timeout"
		result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
		if len(result.Failures) != 1 || result.Failures[0].Outcome != port.Incomplete || len(engine.Networks) != 2 || engine.Resources[r.Egress.Proxy.ID].Running {
			t.Errorf("copy timeout lost stopped proxy: %#v", result)
		}
		synctest.Wait()
		engine.EgressFault = ""
		result = (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
		if len(result.Failures) != 0 || len(engine.Resources) != 0 {
			t.Errorf("copy timeout retry: %#v", result)
		}
	})
}
func TestFiniteEgressCallsRefuseExpiredBudget(t *testing.T) {
	b, _, engine := f.Fixture()
	b = f.EgressBinding(b)
	run := &f.RunEngine{Engine: engine}
	base, failure := run.OpenRun(t.Context(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer base.Close()
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	c := finiteConnection{base, gate}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := f.EgressRecords(b)[1]
	if _, failure := c.CreateNetwork(ctx, r, port.InternalNetwork); failure != port.Uncertain {
		t.Error(failure)
	}
	if _, status := c.InspectNetwork(ctx, r.Egress.Internal.Name); status != port.Unavailable {
		t.Error(status)
	}
	c.RemoveNetwork(ctx, r.Egress.Internal.ID)
	if _, failure := c.CreateProxy(ctx, r); failure != port.Uncertain {
		t.Error(failure)
	}
	if failure := c.VerifyProxy(ctx, r.Egress.Proxy.ID); failure != port.IsolationInspection {
		t.Error(failure)
	}
	if failure := c.ProxyReady(ctx, r.Egress.Proxy.ID); failure != port.ProxyFailed {
		t.Error(failure)
	}
	if failure := c.CopyProxyLog(ctx, r.Egress.Proxy.ID, nil); failure != port.Incomplete {
		t.Error(failure)
	}
}

func TestEgressLateLogAcquisitionIsClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, registry, engine, id, _ := seedEgressSession(t, 9)
		log := runner.Refusals.(*f.RefusalLog)
		release := make(chan struct{})
		log.BeginUncancellable = release
		go func() { time.Sleep(11 * time.Second); close(release) }()
		result := (Recovery{Registry: registry, Engine: engine, Refusals: log}).Stop(t.Context(), id)
		if len(result.Failures) != 1 || result.Failures[0].Outcome != port.Incomplete || len(engine.Resources) != 1 {
			t.Errorf("late begin did not retain proxy: %#v", result)
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if log.LastCopy == nil {
			t.Fatal("late acquisition never returned")
		}
		if _, err := log.LastCopy.Write([]byte("late")); err == nil {
			t.Error("late copy was not closed")
		}
		log.BeginUncancellable = nil
		result = (Recovery{Registry: registry, Engine: engine, Refusals: log}).Stop(t.Context(), id)
		if len(result.Failures) != 0 {
			t.Error("late begin retry", result)
		}
	})
}
