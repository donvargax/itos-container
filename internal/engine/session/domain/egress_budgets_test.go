package domain

import (
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"slices"
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

func TestEgressProxyStopWaitIsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, registry, engine, id, r := seedEgressSession(t, 9)
		engine.EgressFault = "proxy ignores kill"
		start := time.Now()
		result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
		synctest.Wait()
		if time.Since(start) != budget.CallDuration || len(result.Failures) != 1 || result.Failures[0].Outcome != port.Incomplete || !engine.Resources[r.Egress.Proxy.ID].Running || slices.Contains(engine.Events, "proxy copy") || slices.Contains(engine.Events, "proxy remove") {
			t.Fatalf("unconfirmed proxy exit = %#v after %s; %v", result, time.Since(start), engine.Events)
		}
		engine.EgressFault = ""
		if retry := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id); len(retry.Failures) != 0 || len(engine.Resources) != 0 || len(engine.Networks) != 0 || len(engine.Anchors) != 0 {
			t.Fatal("confirmed stop did not release retained resources", retry)
		}
	})
}

func TestEgressCleanupSharesThirtySecondDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, store, engine, ref, before := cleanupFixture(t, 3, "reserved")
		engine.After = func(event string) {
			switch event {
			case "open", "proxy SIGKILL", "proxy copy", "egress-network remove":
				time.Sleep(9 * time.Second)
			}
		}
		start := time.Now()
		result := runner.Cleanup(t.Context(), ref)
		elapsed := time.Since(start)
		// Finish the timed-out remove before inspecting the shared fake state.
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if result.Outcome != port.Incomplete || elapsed != 30*time.Second || store.Locked {
			t.Fatalf("cleanup = %#v after %s, want incomplete at 30s with released lock", result, elapsed)
		}
		saved, failure := store.Codec.Decode(store.Records[ref])
		before.Egress.Proxy.Stage = "absent"
		if failure != "" || saved != before || len(engine.Resources) != 0 || len(engine.Networks) != 2 || len(engine.Anchors) != 1 {
			t.Fatal("expired cleanup released recovery authority", saved, engine.Events)
		}
		engine.After = nil
		if retry := runner.Cleanup(t.Context(), ref); retry.Outcome != port.Complete {
			t.Fatal("fresh cleanup did not finish retained resources", retry)
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
