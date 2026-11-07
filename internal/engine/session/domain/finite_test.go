package domain

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
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

func finiteRegistryPorts() (*porttest.Registry, *porttest.Store, approval.Durable) {
	binding, store, _ := porttest.Fixture()
	return &porttest.Registry{Store: store, Project: binding.Frozen.Project}, store, approval.Durable{Frozen: binding.Frozen}
}

func TestFiniteRegistryWithoutRunBindingPreservesContract(t *testing.T) {
	porttest.RegistryContract(t, func() (port.Registry, port.Store, approval.Durable) {
		registry, store, frozen := finiteRegistryPorts()
		// A registry need not implement RunRegistry. Keep its ordinary contract
		// and finite bounds when selecting it for a run.
		plain := struct{ port.Registry }{registry}
		return (finiteRegistry{plain}).ForRun(frozen), store, frozen
	})
}

func TestFiniteRegistryStopsAtExpiredPhase(t *testing.T) {
	ctx, end := budget.Start(context.Background(), 0, "startup")
	defer end()
	registry, _, _ := finiteRegistryPorts()
	type result struct {
		session port.Session
		failure port.Outcome
	}
	done := make(chan result, 1)
	go func() {
		s, f := (finiteRegistry{registry}).Open(ctx, strings.Repeat("a", 64), false)
		done <- result{s, f}
	}()
	// A typed phase failure must end lock retries too. Keep this guard outside
	// synctest so a retry loop cannot prevent the test clock from advancing.
	select {
	case r := <-done:
		if r.session != nil || r.failure != port.Incomplete {
			t.Fatal("expired phase acquired a session", r)
		}
	case <-time.After(time.Second):
		t.Fatal("session lock acquisition kept retrying an expired phase")
	}
}

func seedFiniteSession(t *testing.T, registry *porttest.Registry, frozen approval.Durable) string {
	t.Helper()
	id, token := strings.Repeat("a", 64), strings.Repeat("1", 64)
	lease, f := registry.Register(context.Background(), frozen, id, token)
	if f != "" {
		t.Fatal(f)
	}
	lease.Close()
	i, f := registry.Store.New(context.Background(), frozen, token)
	if f != "" {
		t.Fatal(f)
	}
	i.Close()
	return id
}

func TestExpiredSessionCallsPreserveRecoveryEvidence(t *testing.T) {
	registry, store, frozen := finiteRegistryPorts()
	id := seedFiniteSession(t, registry, frozen)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finite := finiteRegistry{registry}
	s, f := finite.Open(ctx, id, false)
	if f != "" || s == nil {
		t.Fatal(f)
	}
	defer s.Close()
	i := s.Intent()
	owned := i.Record()
	owned.Stage, owned.ContainerID = "owned", strings.Repeat("2", 64)
	if f := i.Save(owned); f != "" {
		t.Fatal("healthy publication failed", f)
	}
	before := string(store.Records[s.Record().Intent])
	cancel()
	if ids, f := finite.List(ctx); ids != nil || f != port.Incomplete {
		t.Fatal("expired discovery reported success", ids, f)
	}
	if state, f := s.Controller(); state != "unknown" || f != port.Incomplete {
		t.Fatal("expired inspection invented controller state", state, f)
	}
	absent := owned
	absent.Stage = "absent"
	if f := i.Save(absent); f != port.Incomplete {
		t.Fatal("expired publication reported completion", f)
	}
	if i.Record() != owned || string(store.Records[s.Record().Intent]) != before {
		t.Fatal("expired publication changed retained ownership")
	}
}

type delayedSessionRegistry struct {
	port.Registry
	release <-chan struct{}
}

func (r delayedSessionRegistry) Open(ctx context.Context, id string, readOnly bool) (port.Session, port.Outcome) {
	s, f := r.Registry.Open(ctx, id, readOnly)
	<-r.release
	return s, f
}

func TestFiniteRegistryClosesLateSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		registry, _, frozen := finiteRegistryPorts()
		id := seedFiniteSession(t, registry, frozen)
		closes := 0
		registry.AfterClose = func(string) { closes++ }
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		release := make(chan struct{})
		start := time.Now()
		s, f := (finiteRegistry{delayedSessionRegistry{registry, release}}).Open(ctx, id, false)
		if s != nil || f != port.Incomplete || time.Since(start) != 5*time.Second {
			t.Error("pending acquisition exceeded its phase or returned a handle", s, f, time.Since(start))
		}
		synctest.Wait()
		if !registry.Locks[id] || closes != 0 {
			t.Error("pending acquisition lost its lock")
		}
		close(release)
		synctest.Wait()
		if registry.Locks[id] || closes != 1 {
			t.Fatal("late session retained its lock or closed repeatedly", registry.Locks[id], closes)
		}
		// A fresh recovery attempt must be able to acquire the released lock.
		s, f = (finiteRegistry{registry}).Open(context.Background(), id, false)
		if f != "" || s == nil {
			t.Fatal("late session prevented fresh recovery", f)
		}
		s.Close()
	})
}
