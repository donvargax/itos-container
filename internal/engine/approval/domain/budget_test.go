package domain

import (
	"context"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestExpiredApprovalCallsNeverEnterPorts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, f := (boundedStore{ctx: ctx}).Read(port.Candidate{}); f != "state.unavailable" {
		t.Fatal(f)
	}
	if _, f := (boundedStore{ctx: ctx}).Select(port.Candidate{}, "1"); f != "state.unavailable" {
		t.Fatal(f)
	}
	if r := (boundedReadiness{ctx: ctx}).InspectBeforeEngine(func(rp.Result) bool { t.Fatal("entered after cancellation"); return true }); r.Ready {
		t.Fatal(r)
	}
	if v := (Preflight{}).InspectContext(ctx); v.Failure == "" {
		t.Fatal(v)
	}
	if _, f := (Preflight{}).FreezeContext(ctx, port.View{}); f == "" {
		t.Fatal("cancelled freeze")
	}
}
func TestContextApprovalRetainsMatchingSelection(t *testing.T) {
	s, ready, _, store := serviceFixture()
	codec := s.Codec.(*porttest.Codec)
	c := codec.Values["2000:3000"]
	store.Records = map[string]port.Record{c.Project: {Candidate: c, Generation: "7"}}
	p := Preflight{Service: s, Local: ready, Canonical: codec}
	v := p.InspectContext(context.Background())
	if v.Status != "matching" {
		t.Fatal(v)
	}
	if frozen, f := p.FreezeContext(context.Background(), v); f != "" || frozen.Generation != "7" {
		t.Fatal(frozen, f)
	}
}
