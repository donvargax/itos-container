package domain

import (
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestHomeRetirementRevalidatesEveryDestructiveBoundary(t *testing.T) {
	for _, fault := range []string{"missing store", "read", "missing generation", "current", "uid", "gid", "engine", "tuple", "stage", "successor", "fresh unavailable", "presence changed", "completed present", "engine open", "engine check", "inspect foreign", "reread unavailable", "reread changed", "late claim", "late engine check", "remove changed"} {
		t.Run(fault, func(t *testing.T) {
			x := domainHomeFixture()
			g := x.seed()
			s := x.store.Scopes[x.store.Scope(x.b)]
			e := homeEntry(s, g)
			e.State = "present"
			want := port.Refused
			completed := false
			switch fault {
			case "missing store":
				x.store.MissingOpen = true
			case "read":
				x.store.ReadFailure = port.Unsafe
				want = port.Unsafe
			case "missing generation":
				s.Generations = nil
			case "current":
				s.Current = "changed"
			case "uid":
				s.UID = "42"
			case "gid":
				s.GID = "42"
			case "engine":
				s.Engine.ID = "changed"
			case "tuple":
				s.Generations[0].Created = "changed"
			case "stage":
				s.Generations[0].Stage = "deleted"
			case "successor":
				s.Generations[0].Successor = "changed"
			case "fresh unavailable":
				x.engine.HomeFault = "inspect"
				want = port.Incomplete
			case "presence changed":
				delete(x.engine.Homes, g.Name)
			case "completed present":
				completed = true
			case "engine open", "engine check":
				opens := 0
				x.engine.AfterOpen = func() {
					opens++
					if opens == 2 {
						x.engine.Unavailable = true
					}
				}
				if fault == "engine open" {
					x.store.BeforeSave = func(port.HomeScope) port.Outcome { x.engine.Unavailable = true; return "" }
				}
				want = port.Incomplete
			default:
				inspects := 0
				x.engine.HomeBefore = func(event string) {
					if event != "inspect" {
						return
					}
					inspects++
					if inspects == 2 {
						switch fault {
						case "inspect foreign":
							o := x.engine.Homes[g.Name]
							o.Created = "changed"
							x.engine.Homes[g.Name] = o
						case "reread unavailable":
							x.store.ReadFailure = port.Unsafe
						case "reread changed":
							q := x.store.Scopes[s.Scope]
							q.Generations[0].Initialized = false
							x.store.Scopes[s.Scope] = q
						case "late claim":
							x.store.ClaimsFailure = port.HomeInUse
						case "late engine check":
							x.engine.AfterOpen = nil
							x.engine.Unavailable = true
						}
					}
					if fault == "remove changed" && inspects == 3 {
						x.engine.Replaced = true
					}
				}
				if fault == "reread unavailable" {
					want = port.Unsafe
				}
				if fault == "late claim" {
					want = port.HomeInUse
				}
				if fault == "late engine check" {
					want = port.Incomplete
				}
			}
			reviewed := x.store.Scopes[x.store.Scope(x.b)]
			// Maps contain slice-backed generations; preserve an independent reviewed tuple.
			reviewed.Generations = []port.HomeGeneration{g}
			reviewed.Current = g.ID
			reviewed.UID = x.b.UID
			reviewed.GID = x.b.GID
			reviewed.Engine = port.HomeEngineIdentity{Endpoint: x.b.Endpoint, ID: x.b.EngineID}
			x.store.Scopes[s.Scope] = s
			result := x.h.retire(t.Context(), reviewed, g, e, "delete", completed)
			if got := homeOutcome(result); got != want {
				t.Fatalf("retire %+v want %s", result, want)
			}
			if fault != "remove changed" && slices.Contains(x.engine.HomeEvents, "remove") {
				t.Fatal("unproven ownership removed volume")
			}
		})
	}
}

func TestHomeCatalogOrdersAllGenerationsAndReportsObservationFailures(t *testing.T) {
	for _, fault := range []string{"sorted", "changed", "engine check", "no capability", "unissued", "allocation absent"} {
		t.Run(fault, func(t *testing.T) {
			x := domainHomeFixture()
			g := x.seed()
			s := x.store.Scopes[x.store.Scope(x.b)]
			want := port.Outcome("")
			switch fault {
			case "sorted":
				later := g
				later.ID = strings.Repeat("f", 64)
				later.Name = "itos-container-home-" + later.ID
				later.Stage = "deleted"
				s.Generations = []port.HomeGeneration{later, g}
			case "changed":
				x.engine.HomeBefore = func(string) { x.engine.Replaced = true }
				want = port.Refused
			case "engine check":
				x.engine.AfterOpen = func() { x.engine.Replaced = true }
				want = port.Refused
			case "no capability":
				x.h.Engine = x.engine.RunEngine
				want = port.Incomplete
			case "unissued":
				s.Generations[0].Created = ""
				s.Generations[0].Stage = "awaiting-allocation"
				want = port.Refused
			case "allocation absent":
				s.Generations[0].Stage = "intent"
				s.Generations[0].Created = ""
				delete(x.engine.Homes, g.Name)
				want = port.Incomplete
			}
			x.store.Scopes[s.Scope] = s
			result := x.h.ListHomes(t.Context())
			if homeOutcome(result) != want {
				t.Fatal(result, want)
			}
			if fault == "sorted" && (len(result.Homes) != 2 || result.Homes[0].ID != g.ID || result.Homes[1].ID != strings.Repeat("f", 64)) {
				t.Fatal(result)
			}
		})
	}
}

func TestHomeChangedRemovalTupleCannotReleaseSessionClaim(t *testing.T) {
	x := domainHomeFixture()
	g := x.seed()
	entry := x.session(t, &g, "reserved")
	x.engine.HomeBefore = func(string) { x.engine.Replaced = true }
	c, _ := x.engine.OpenRun(t.Context(), x.b)
	defer c.Close()
	state, failure := x.h.reconcile(t.Context(), entry, c, true)
	if failure != port.Refused || state != "unknown" {
		t.Fatal(state, failure)
	}
}
