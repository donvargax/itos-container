package domain

import (
	"context"
	"testing"
	"time"

	mp "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestExpiredReadinessCallsNeverEnterPorts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := boundedCheckout{ctx: ctx}
	if c.Pin("root") != port.CheckoutUnavailable || c.Unchanged() {
		t.Fatal("expired checkout")
	}
	if _, f := c.Inspect(); f != port.CheckoutUnavailable {
		t.Fatal(f)
	}
	if _, issues := (boundedPolicy{ctx: ctx}).Read(); issues != nil {
		t.Fatal(issues)
	}
	if _, f := (boundedFiles{ctx: ctx}).Read("manifest"); f != mp.Path {
		t.Fatal(f)
	}
	s := boundedSession{ctx: ctx}
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
	if v, f := (boundedEngine{ctx: ctx}).Open("", "", time.Second); v != nil || f != port.EngineTimeout {
		t.Fatal(v, f)
	}
	if r := (Inspector{}).InspectBeforeEngineContext(ctx, port.Input{}, nil); r.Ready || len(r.Issues) != 1 {
		t.Fatal(r)
	}
}
func TestContextReadinessUsesRealInspection(t *testing.T) {
	i, input, _, _ := fixture()
	r := i.InspectBeforeEngineContext(context.Background(), input, func(r port.Result) bool { return r.Engine == nil })
	if !r.Ready {
		t.Fatal(r)
	}
}
