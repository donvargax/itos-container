package domain

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func (x homeFixture) session(t *testing.T, g *port.HomeGeneration, stage string) port.Session {
	t.Helper()
	id, token := strings.Repeat("1", 64), strings.Repeat("2", 64)
	i, failure := x.registry.Store.Prepare(t.Context(), x.frozen, token)
	if failure != "" {
		t.Fatal(failure)
	}
	r := i.Record()
	if g != nil {
		r.Home = g.Reference(x.store.Scope(x.b))
		if failure = i.Save(r); failure != "" {
			t.Fatal(failure)
		}
	}
	if stage != "reserved" {
		r.Stage = "intent"
		if failure = i.Save(r); failure != "" {
			t.Fatal(failure)
		}
		r.Stage = stage
		r.ContainerID = strings.Repeat("c", 64)
		if failure = i.Save(r); failure != "" {
			t.Fatal(failure)
		}
	}
	i.Close()
	l, failure := x.registry.Register(t.Context(), x.frozen, id, token)
	if failure != "" {
		t.Fatal(failure)
	}
	l.Close()
	entry, failure := x.registry.Open(t.Context(), id, false)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(entry.Close)
	return entry
}

func TestHomePreparationStopsAtUncertainAuthority(t *testing.T) {
	for _, fault := range []string{"missing transaction", "read", "initial save", "scope binding", "missing current", "intent save", "claim", "claim save", "no capability", "occupied", "late unavailable", "late foreign", "retained unavailable"} {
		t.Run(fault, func(t *testing.T) {
			x := domainHomeFixture()
			entry := x.session(t, nil, "reserved")
			c, failure := x.engine.OpenRun(t.Context(), x.b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			want := port.HomeUnavailable
			switch fault {
			case "missing transaction":
				x.store.MissingOpen = true
			case "read":
				x.store.ReadFailure = port.Incomplete
			case "initial save":
				x.store.BeforeSave = func(port.HomeScope) port.Outcome { return port.IntentWrite }
			case "scope binding", "missing current":
				x.seed()
				s := x.store.Scopes[x.store.Scope(x.b)]
				if fault == "scope binding" {
					s.GID = "42"
				} else {
					s.Current = strings.Repeat("f", 64)
				}
				x.store.Scopes[s.Scope] = s
				want = port.HomeRefused
			case "intent save":
				x.registry.Store.Fault = "reserved-record write"
			case "claim":
				x.store.ClaimFailure = port.Invalid
				want = port.HomeRefused
			case "claim save":
				x.store.BeforeSave = func(s port.HomeScope) port.Outcome {
					if len(s.Generations[0].Claims) > 0 {
						return port.IntentWrite
					}
					return ""
				}
			case "no capability":
				c, _ = x.engine.RunEngine.OpenRun(t.Context(), x.b)
			case "occupied":
				x.engine.Homes["itos-container-home-"+strings.Repeat("e", 64)] = port.HomeObservation{}
				want = port.HomeRefused
			case "late unavailable", "late foreign":
				inspects := 0
				x.engine.HomeBefore = func(event string) {
					if event == "inspect" {
						inspects++
						if inspects == 2 {
							if fault == "late unavailable" {
								x.engine.HomeFault = "inspect"
							} else {
								for name, o := range x.engine.Homes {
									o.Token = "foreign"
									x.engine.Homes[name] = o
								}
							}
						}
					}
				}
				if fault == "late foreign" {
					want = port.HomeRefused
				}
			case "retained unavailable":
				x.seed()
				x.engine.HomeFault = "inspect"
			}
			if got := x.h.prepare(t.Context(), entry, c); got != want {
				t.Fatalf("prepare %s want %s", got, want)
			}
			if slices.Contains(x.engine.Events, "start") || slices.Contains(x.engine.HomeEvents, "remove") {
				t.Fatal("uncertain preparation mutated workload")
			}
		})
	}
}

func TestHomeVerificationCannotRepairRetainedOrChangedRoots(t *testing.T) {
	for _, fault := range []string{"open", "missing", "read", "current", "claim", "tuple", "no capability", "absent", "unavailable", "foreign", "uninitialized", "root", "publication", "valid"} {
		t.Run(fault, func(t *testing.T) {
			x := domainHomeFixture()
			g := x.seed()
			entry := x.session(t, &g, "owned")
			s := x.store.Scopes[x.store.Scope(x.b)]
			s.Generations[0].Claims = []port.HomeClaim{{Session: entry.Record().ID}}
			x.store.Scopes[s.Scope] = s
			x.engine.Workers[entry.Intent().Record().ContainerID] = g.Name
			c, _ := x.engine.OpenRun(t.Context(), x.b)
			defer c.Close()
			want := port.HomeRefused
			initialize := false
			switch fault {
			case "open":
				x.store.Failure = port.Incomplete
				want = port.HomeUnavailable
			case "missing":
				x.store.MissingOpen = true
			case "read":
				x.store.ReadFailure = port.Unsafe
			case "current":
				s.Current = "other"
			case "claim":
				s.Generations[0].Claims = nil
			case "tuple":
				s.Generations[0].Created = "changed"
			case "no capability":
				c, _ = x.engine.RunEngine.OpenRun(t.Context(), x.b)
				want = port.HomeUnavailable
			case "absent":
				delete(x.engine.Homes, g.Name)
			case "unavailable":
				x.engine.HomeFault = "inspect"
				want = port.HomeUnavailable
			case "foreign":
				o := x.engine.Homes[g.Name]
				o.Token = "other"
				x.engine.Homes[g.Name] = o
			case "uninitialized":
				s.Generations[0].Initialized = false
			case "root":
				r := x.engine.Roots[g.Name]
				r.Directory = false
				x.engine.Roots[g.Name] = r
			case "publication":
				s.Generations[0].Initialized = false
				initialize = true
				x.store.BeforeSave = func(port.HomeScope) port.Outcome { return port.IntentWrite }
				want = port.HomeUnavailable
			case "valid":
				want = ""
			}
			x.store.Scopes[s.Scope] = s
			if got := x.h.verify(t.Context(), entry, c, initialize); got != want {
				t.Fatalf("verify %s want %s", got, want)
			}
			if !initialize && slices.Contains(x.engine.HomeEvents, "initialize") {
				t.Fatal("repaired retained root")
			}
		})
	}
}

func TestHomeReconciliationPreservesUncertainAllocation(t *testing.T) {
	for _, fault := range []string{"open", "missing", "read", "generation", "tuple", "no capability", "pending observed", "pending absent", "unissued observed", "reference publication", "claim publication", "absent"} {
		t.Run(fault, func(t *testing.T) {
			x := domainHomeFixture()
			g := x.seed()
			original := g
			if strings.HasPrefix(fault, "pending") || fault == "reference publication" {
				g.Created = ""
				g.Stage = "intent"
				g.Initialized = false
				s := x.store.Scopes[x.store.Scope(x.b)]
				s.Generations[0] = g
				x.store.Scopes[s.Scope] = s
			}
			entry := x.session(t, &g, "reserved")
			c, _ := x.engine.OpenRun(t.Context(), x.b)
			defer c.Close()
			release := false
			want := port.Incomplete
			s := x.store.Scopes[x.store.Scope(x.b)]
			switch fault {
			case "open":
				x.store.Failure = port.Incomplete
			case "missing":
				x.store.MissingOpen = true
				want = port.Invalid
			case "read":
				x.store.ReadFailure = port.Unsafe
				want = port.Unsafe
			case "generation":
				s.Generations = nil
				want = port.Invalid
			case "tuple":
				s.Generations[0].Created = "changed"
				want = port.Refused
			case "no capability":
				c, _ = x.engine.RunEngine.OpenRun(t.Context(), x.b)
			case "pending absent":
				delete(x.engine.Homes, g.Name)
			case "unissued observed":
				s.Generations[0].Stage = "awaiting-allocation"
				want = port.Refused
			case "reference publication":
				release = true
				x.registry.Store.Fault = "reserved-record write"
				want = port.IntentWrite
			case "claim publication":
				release = true
				i := entry.Intent().Record()
				i.Stage = "cancelled"
				if f := entry.Intent().Save(i); f != "" {
					t.Fatal(f)
				}
				x.store.BeforeSave = func(port.HomeScope) port.Outcome { return port.IntentWrite }
				want = port.IntentWrite
			case "absent":
				delete(x.engine.Homes, g.Name)
				want = ""
			}
			x.store.Scopes[s.Scope] = s
			state, got := x.h.reconcile(t.Context(), entry, c, release)
			if got != want {
				t.Fatalf("reconcile %s %s want %s", state, got, want)
			}
			if slices.Contains(x.engine.HomeEvents, "allocate") || slices.Contains(x.engine.HomeEvents, "remove") || slices.Contains(x.engine.HomeEvents, "initialize") {
				t.Fatal("recovery changed home contents")
			}
			if fault == "pending observed" {
				x.engine.Homes[g.Name] = port.HomeObservation{Name: original.Name, Created: original.Created, Driver: "local", Scope: "local", Token: original.ID, Key: s.Scope, ExactLabels: true, EmptyOptions: true}
				if _, got = x.h.reconcile(t.Context(), entry, c, true); got != "" || entry.Intent().Record().Home.Created != original.Created {
					t.Fatal("matching delayed allocation not accounted for", got)
				}
			}
		})
	}
}

func TestHomeCatalogNeverReturnsPartialTrustedState(t *testing.T) {
	for _, fault := range []string{"open", "missing", "read"} {
		t.Run(fault, func(t *testing.T) {
			x := domainHomeFixture()
			g := x.seed()
			want := port.Invalid
			switch fault {
			case "open":
				x.store.Before = func(event string) {
					if event == "open" {
						x.store.Failure = port.Unsafe
					}
				}
				want = port.Unsafe
			case "missing":
				x.store.MissingOpen = true
			case "read":
				x.store.ReadFailure = port.Unsafe
				want = port.Unsafe
			}
			if r := x.h.ListHomes(t.Context()); homeOutcome(r) != want || len(r.Homes) != 0 {
				t.Fatal(r)
			}
			x.store.Failure = ""
			if r := x.h.Retire(t.Context(), g.ID, "reset", f.HomeConsent{Yes: true}); homeOutcome(r) != want || len(r.Homes) != 0 {
				t.Fatal(r)
			}
		})
	}
}

func TestHomeRecoveryMountMismatchRequiresAnUnstartedWorker(t *testing.T) {
	for _, state := range []string{"unstarted", "started", "pid", "running", "exited", "foreign"} {
		t.Run(state, func(t *testing.T) {
			x := domainHomeFixture()
			g := x.seed()
			entry := x.session(t, &g, "owned")
			r := entry.Intent().Record()
			o := port.Observation{ID: r.ContainerID, Name: r.Name, ImageID: r.ImageID,
				Token: r.NameToken(), Project: r.Project, Fingerprint: r.Fingerprint,
				StatePresent: true, Status: "created"}
			// Both mount observations are deliberately untrusted. Only an exact
			// worker which never started can be removed without trusting them.
			switch state {
			case "started":
				o.Started = true
			case "pid":
				o.PID = 123
			case "running":
				o.Running = true
			case "exited":
				o.Status = "exited"
			case "foreign":
				o.Token = strings.Repeat("f", 64)
			}
			x.engine.Resources[o.ID] = o
			c, f := x.engine.OpenRun(t.Context(), x.b)
			if f != "" {
				t.Fatal(f)
			}
			defer c.Close()
			_, resource, failure := inspectSession(t.Context(), entry, c)
			if state == "unstarted" {
				if failure != "" || resource != "created" {
					t.Fatal("unstarted owned worker is not reclaimable", resource, failure)
				}
			} else if failure != port.Refused || resource != "unknown" {
				t.Fatal("untrusted mount observation authorized recovery", resource, failure)
			}
		})
	}
}

type lateHomeStore struct {
	port.HomeStore
	release <-chan struct{}
}

func (s lateHomeStore) Open(ctx context.Context, key string, create bool) (port.HomeTransaction, port.Outcome) {
	t, f := s.HomeStore.Open(ctx, key, create)
	<-s.release
	return t, f
}

func TestHomeFiniteOperationsDiscardLateReplies(t *testing.T) {
	for _, operation := range []string{"list", "claims", "read", "open", "inspect", "allocate", "missing capability"} {
		synctest.Test(t, func(t *testing.T) {
			x := domainHomeFixture()
			g := x.seed()
			release := make(chan struct{})
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			start := time.Now()
			switch operation {
			case "list", "claims":
				x.store.Before = func(event string) {
					if event == operation {
						<-release
					}
				}
				if operation == "list" {
					if _, f := x.h.list(ctx); f != port.Incomplete {
						t.Fatal(f)
					}
				} else {
					if f := x.h.checkClaims(ctx, x.store.Scopes[x.store.Scope(x.b)], g); f != port.Incomplete {
						t.Fatal(f)
					}
				}
			case "open":
				x.h.Store = lateHomeStore{x.store, release}
				if handle, f := x.h.open(ctx, x.store.Scope(x.b), false); handle != nil || f != port.HomeUnavailable {
					t.Fatal(handle, f)
				}
			case "read":
				tx, failure := x.h.open(ctx, x.store.Scope(x.b), false)
				if failure != "" {
					t.Fatal(failure)
				}
				x.store.Before = func(event string) {
					if event == "read" {
						<-release
					}
				}
				if _, f := tx.Read(); f != port.HomeUnavailable {
					t.Fatal(f)
				}
				tx.Close()
			default:
				var engine port.RunEngine = x.engine
				if operation == "missing capability" {
					engine = x.engine.RunEngine
				} else {
					x.engine.HomeBefore = func(event string) {
						if event == operation {
							<-release
						}
					}
				}
				c, failure := (finiteEngine{engine}).OpenRun(ctx, x.b)
				if failure != "" {
					t.Fatal(failure)
				}
				hc := c.(port.HomeConnection)
				if operation == "inspect" || operation == "missing capability" {
					if _, state := hc.InspectHome(ctx, g.Name); state != port.Unavailable {
						t.Fatal(state)
					}
				}
				if operation != "inspect" {
					want := port.HomeUnavailable
					if operation == "missing capability" {
						// Without the home capability no request can be issued.
						want = port.HomeNotIssued
					}
					if f := hc.AllocateHome(ctx, g.Reference(x.store.Scope(x.b))); f != want {
						t.Fatal(f)
					}
				}
				c.Close()
			}
			if operation != "missing capability" && time.Since(start) != time.Second {
				t.Fatal(operation, time.Since(start))
			}
			close(release)
			synctest.Wait()
			x.store.Before = nil
			if operation == "read" || operation == "open" {
				tx, failure := x.store.Open(t.Context(), x.store.Scope(x.b), false)
				if failure != "" || tx == nil {
					t.Fatal("late lock leaked", failure)
				}
				tx.Close()
			}
		})
	}
}
