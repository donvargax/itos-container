package domain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	mp "github.com/donvargax/itos-container/internal/engine/manifest/port"
	manifesttest "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	policytest "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

func TestExpiredReadinessCallsNeverEnterPorts(t *testing.T) {
	synctest.Test(t, testExpiredReadinessCallsNeverEnterPorts)
}

func testExpiredReadinessCallsNeverEnterPorts(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Ports and the checkout gate are initialized just as in a real assembly.
	// Cancellation forbids operations, but interrupted Pin may still Close.
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	checkout := &porttest.Checkout{Opened: true}
	policy := &policytest.Source{}
	files := &manifesttest.Files{}
	session := &porttest.Session{}
	engine := &porttest.Engine{Session: session}
	c := boundedCheckout{Checkout: checkout, ctx: ctx, gate: gate}
	if c.Pin("root") != port.CheckoutUnavailable || c.Unchanged() {
		t.Fatal("expired checkout")
	}
	if _, f := c.Inspect(); f != port.CheckoutUnavailable {
		t.Fatal(f)
	}
	if _, issues := (boundedPolicy{Source: policy, ctx: ctx, gate: gate}).Read(); issues != nil {
		t.Fatal(issues)
	}
	if _, f := (boundedFiles{Files: files, ctx: ctx, gate: gate}).Read("manifest"); f != mp.Path {
		t.Fatal(f)
	}
	s := boundedSession{Session: session, ctx: ctx}
	if s.Unchanged() {
		t.Fatal("expired socket")
	}
	if _, f := s.Version(); f != port.EngineTimeout {
		t.Fatal(f)
	}
	if _, f := s.Info(); f != port.EngineTimeout {
		t.Fatal(f)
	}
	if _, f := s.Image("ref"); f != port.EngineTimeout {
		t.Fatal(f)
	}
	if v, f := (boundedEngine{Engine: engine, ctx: ctx}).Open("", "", time.Second); v != nil || f != port.EngineTimeout {
		t.Fatal(v, f)
	}
	i := Inspector{Checkout: checkout, Policy: policy, Files: files, Engine: engine}
	if r := i.InspectBeforeEngineContext(ctx, port.Input{}, nil); r.Ready || len(r.Issues) != 1 || r.Issues[0].Kind != port.CheckoutUnavailable {
		t.Fatal(r)
	}
	synctest.Wait()
	if len(checkout.Roots) != 0 || checkout.Inspections != 0 || checkout.Revalidations != 0 {
		t.Fatalf("cancelled checkout entered a port: %+v", checkout)
	}
	if policy.Reads != 0 || len(files.Reads) != 0 {
		t.Fatalf("cancelled readers entered a port: policy %d, manifest %v", policy.Reads, files.Reads)
	}
	if engine.Opens != 0 || session.VersionReads != 0 || session.InfoReads != 0 || len(session.ImageReads) != 0 || session.Revalidations != 0 {
		t.Fatalf("cancelled engine entered a port: opens %d, session %+v", engine.Opens, session)
	}
}
func TestContextReadinessUsesRealInspection(t *testing.T) {
	i, input, _, _ := fixture()
	r := i.InspectBeforeEngineContext(context.Background(), input, func(r port.Result) bool { return r.Engine == nil })
	if !r.Ready {
		t.Fatal(r)
	}
}

func TestBoundedEnginePreservesOpenFailure(t *testing.T) {
	engine := &porttest.Engine{Failure: port.EngineUnavailable}
	session, failure := (boundedEngine{engine, context.Background()}).Open("local", "checkout", time.Hour)
	if session != nil || failure != port.EngineUnavailable || engine.Opens != 1 {
		t.Fatal("failed connection became usable or lost its cause", session, failure, engine.Opens)
	}
}

func TestBoundedLegacyEngineReceivesRemainingBudget(t *testing.T) {
	for _, remaining := range []time.Duration{5 * time.Second, 120 * time.Second} {
		t.Run(remaining.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, end := budget.Start(context.Background(), remaining, "startup")
				defer end()
				session := porttest.HealthySession()
				engine := &porttest.Engine{Session: session}
				// Expose the legacy port only, retaining the same in-memory engine.
				legacy := struct{ port.Engine }{engine}
				s, failure := (boundedEngine{legacy, ctx}).Open("local", "checkout", time.Hour)
				if failure != port.None || s == nil {
					t.Fatal("healthy legacy connection failed", failure)
				}
				defer s.Close()
				if engine.Budget != min(10*time.Second, remaining) || engine.Selection != "local" || engine.Root != "checkout" {
					t.Fatal("legacy connection lost its selection or finite budget", engine)
				}
				if version, f := s.Version(); f != port.None || version != session.VersionData {
					t.Fatal("establishment expired a healthy session", version, f)
				}
			})
		})
	}
}

type delayedReadinessEngine struct {
	port.Engine
	release <-chan struct{}
}

func (e delayedReadinessEngine) Open(selection, root string, limit time.Duration) (port.Session, port.Failure) {
	s, f := e.Engine.Open(selection, root, limit)
	<-e.release
	return s, f
}

func TestBoundedEngineClosesLateConnection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, end := budget.Start(context.Background(), 120*time.Second, "startup")
		defer end()
		session := porttest.HealthySession()
		engine := &porttest.Engine{Session: session}
		release := make(chan struct{})
		start := time.Now()
		s, failure := (boundedEngine{delayedReadinessEngine{engine, release}, ctx}).Open("local", "checkout", time.Hour)
		if s != nil || failure != port.EngineTimeout || time.Since(start) != 10*time.Second {
			t.Error("late connection extended the call or became usable", s, failure, time.Since(start))
		}
		if session.Closed {
			t.Error("connection closed while acquisition was pending")
		}
		close(release)
		synctest.Wait()
		if !session.Closed || engine.Opens != 1 {
			t.Fatal("late connection leaked or acquisition retried", session.Closed, engine.Opens)
		}
	})
}
