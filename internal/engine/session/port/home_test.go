package port_test

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestHomeConsentFakeContract(t *testing.T) {
	f.HomeConsentContract(t, func(answer bool) port.HomeConsent { return f.HomeConsent{Yes: answer} })
}

func TestHomeOwnershipRequiresTheFullOriginalTuple(t *testing.T) {
	h := port.HomeReference{Scope: strings.Repeat("a", 64), ID: strings.Repeat("b", 64), Name: "itos-container-home-" + strings.Repeat("b", 64), Created: "original"}
	o := port.HomeObservation{Name: h.Name, Created: h.Created, Driver: "local", Scope: "local", Token: h.ID, Key: h.Scope, ExactLabels: true, EmptyOptions: true}
	if !h.Valid() || !h.Owns(o) {
		t.Fatal("valid tuple refused")
	}
	for _, change := range []func(*port.HomeObservation){
		func(o *port.HomeObservation) { o.Name = "foreign" }, func(o *port.HomeObservation) { o.Created = "" }, func(o *port.HomeObservation) { o.Created = "replacement" }, func(o *port.HomeObservation) { o.Driver = "nfs" }, func(o *port.HomeObservation) { o.Scope = "global" }, func(o *port.HomeObservation) { o.Token = strings.Repeat("c", 64) }, func(o *port.HomeObservation) { o.Key = strings.Repeat("c", 64) }, func(o *port.HomeObservation) { o.ExactLabels = false }, func(o *port.HomeObservation) { o.EmptyOptions = false },
	} {
		bad := o
		change(&bad)
		if h.Owns(bad) {
			t.Fatalf("adopted %+v", bad)
		}
	}
	for _, change := range []func(*port.HomeReference){func(h *port.HomeReference) { h.ID = "bad" }, func(h *port.HomeReference) { h.Scope = "bad" }, func(h *port.HomeReference) { h.Name = "foreign" }} {
		bad := h
		change(&bad)
		if bad.Valid() || bad.Owns(o) {
			t.Fatal("invalid reference authorized", bad)
		}
	}
	h.Created = ""
	if !h.Owns(o) {
		t.Fatal("matching pending create cannot reconcile")
	}
	noTime := o
	noTime.Created = ""
	if h.Owns(noTime) {
		t.Fatal("name and labels alone conferred ownership")
	}
}

func TestHomeReferenceTransitionsCannotRetargetASession(t *testing.T) {
	b, _, _ := f.Fixture()
	b.Home = true
	r := b.Intent(strings.Repeat("1", 64))
	r.Stage = "reserved"
	h := port.HomeReference{Scope: strings.Repeat("a", 64), ID: strings.Repeat("b", 64), Name: "itos-container-home-" + strings.Repeat("b", 64)}
	bound := r
	bound.Home = h
	if !r.ValidTransition(bound) || !bound.ValidHome(b) {
		t.Fatal("reserved session cannot bind home")
	}
	for _, stage := range []string{"intent", "owned", "absent", "cancelled"} {
		old := r
		old.Stage = stage
		next := old
		next.Home = h
		if old.ValidTransition(next) {
			t.Fatal("late home acquisition", stage)
		}
	}
	for _, stage := range []string{"reserved", "owned", "absent", "cancelled"} {
		old := bound
		old.Stage = stage
		if stage == "owned" || stage == "absent" {
			old.ContainerID = strings.Repeat("c", 64)
		}
		next := old
		next.Home.Created = "original"
		if !old.ValidTransition(next) {
			t.Fatal("cannot finish original allocation", stage)
		}
		if !next.ValidHome(b) {
			t.Fatal("concrete home refused", stage)
		}
		if old.ValidHome(b) != (stage == "reserved" || stage == "cancelled") {
			t.Fatal("unaccounted allocation admitted", stage)
		}
		for _, change := range []func(*port.HomeReference){func(h *port.HomeReference) { h.Created = "replacement" }, func(h *port.HomeReference) { h.ID = strings.Repeat("f", 64) }, func(h *port.HomeReference) { h.Scope = strings.Repeat("f", 64) }, func(h *port.HomeReference) { *h = port.HomeReference{} }} {
			bad := next
			change(&bad.Home)
			if next.ValidTransition(bad) {
				t.Fatal("retargeted home", stage, bad)
			}
		}
	}
	legacy := b
	legacy.Home = false
	if bound.ValidHome(legacy) {
		t.Fatal("home accepted without a grant")
	}
	for _, stage := range []string{"reserved", "cancelled", "intent", "owned"} {
		noHome := r
		noHome.Stage = stage
		if !noHome.ValidHome(legacy) || noHome.ValidHome(b) != (stage == "reserved" || stage == "cancelled") {
			t.Fatal(stage)
		}
	}
}
