package domain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

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
