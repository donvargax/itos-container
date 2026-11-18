package main

import (
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	"strings"
	"testing"
)

func TestHomeSDKRejectsUnprovenArgumentsAndLatePinChanges(t *testing.T) {
	t.Setenv("SLICE_34_HOME_BACKEND", "disk")
	f := newHomeLocalFixture(t, "1000", "1000")
	_, worker, g := f.seedHomeSession(t, "unstarted")
	c, failure := f.runEngine().OpenRun(t.Context(), f.m.b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	h := c.(sp.HomeConnection)
	fresh := g.Reference(f.scope().Scope)
	fresh.Created = ""
	for _, r := range []sp.HomeReference{{}, g.Reference(f.scope().Scope)} {
		if got := h.AllocateHome(t.Context(), r); got != sp.HomeNotIssued {
			t.Fatal("unsafe allocation", got)
		}
	}
	for _, ids := range [][3]string{{"invalid", "1000", "1000"}, {worker, "0", "1000"}, {worker, "bad", "1000"}, {worker, "1000", "bad"}} {
		if got := h.InitializeHome(t.Context(), ids[0], ids[1], ids[2]); got != sp.HomeRefused {
			t.Fatal("invalid root identity", got)
		}
	}
	r := f.m.b.Intent(strings.Repeat("c", 64))
	r.Home = sp.HomeReference{ID: "invalid"}
	if _, got := c.CreateRun(t.Context(), r, []string{"probe"}); got != sp.HomeRefused {
		t.Fatal("invalid home reached create", got)
	}
	f.engine.Replaced = true
	if got := h.AllocateHome(t.Context(), fresh); got != sp.HomeNotIssued {
		t.Fatal(got)
	}
	if _, got := h.InspectHome(t.Context(), g.Name); got != sp.Changed {
		t.Fatal(got)
	}
	if got := h.VerifyHomeRoot(t.Context(), worker, "1000", "1000"); got != sp.HomeRefused {
		t.Fatal(got)
	}
	if got := h.RemoveHome(t.Context(), g.Name); got != sp.Refused {
		t.Fatal(got)
	}
	f.engine.Replaced = false
	for _, fault := range []string{"root unavailable", "root archive"} {
		f.engine.HomeFault = fault
		if got := h.VerifyHomeRoot(t.Context(), worker, "1000", "1000"); got != sp.HomeUnavailable {
			t.Fatal(fault, got)
		}
	}
	f.engine.HomeFault = ""
	if got := h.InitializeHome(t.Context(), worker, "4294967295", "4294967295"); got != "" {
		t.Fatal("maximum numeric archive IDs", got)
	}
	if got := h.VerifyHomeRoot(t.Context(), worker, "4294967295", "4294967295"); got != "" {
		t.Fatal("numeric archive IDs did not round trip", got)
	}
	for _, event := range []string{"inspect", "remove"} {
		f.engine.HomeBefore = func(seen string) {
			if seen == event {
				f.engine.Replaced = true
			}
		}
		if event == "inspect" {
			if _, got := h.InspectHome(t.Context(), g.Name); got != sp.Changed {
				t.Fatal(got)
			}
		} else {
			if got := h.RemoveHome(t.Context(), g.Name); got != sp.Refused {
				t.Fatal(got)
			}
		}
		f.engine.Replaced = false
	}
}

func TestHomeStatusArgumentsRefuseBeforeDiscovery(t *testing.T) {
	f := newHomeLocalFixture(t, "1000", "1000")
	code, out, d := f.command(t, "status", "--homes", "unexpected", "--json")
	if code != 2 || !strings.Contains(d, "recovery.usage") || len(f.engine.Events) != 0 || len(f.homes.Scopes) != 0 {
		t.Fatal(code, out, d)
	}
}
