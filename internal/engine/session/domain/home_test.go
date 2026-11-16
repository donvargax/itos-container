package domain

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type homeFixture struct {
	h        Homes
	runner   Runner
	registry *f.Registry
	store    *f.Homes
	engine   *f.HomeEngine
	b        port.Binding
	frozen   approval.Durable
}

func domainHomeFixture() homeFixture {
	runner, frozen, registry, engine, _ := registeredLifecycleFixture()
	b := registry.Store.Codec.(*f.Codec).Value
	b.Home = true
	registry.Store.Codec.(*f.Codec).Value = b
	store := &f.Homes{Codec: &f.HomeCodec{}}
	homeEngine := &f.HomeEngine{RunEngine: engine, Homes: map[string]port.HomeObservation{}, Roots: map[string]f.HomeRoot{}, Workers: map[string]string{}}
	h := Homes{Store: store, Engine: homeEngine, Entropy: f.Entropy{Value: strings.Repeat("e", 64)}}
	runner.Homes = &h
	runner.Engine = homeEngine
	return homeFixture{h, runner, registry, store, homeEngine, b, frozen}
}
func (x homeFixture) seed() port.HomeGeneration {
	id := strings.Repeat("a", 64)
	key := x.store.Scope(x.b)
	g := port.HomeGeneration{ID: id, Name: "itos-container-home-" + id, Created: "original", Stage: "retained", Initialized: true, Claims: []port.HomeClaim{}}
	x.store.Scopes = map[string]port.HomeScope{key: {Schema: 1, Scope: key, Project: x.b.Frozen.Project, UID: x.b.UID, GID: x.b.GID, Engine: port.HomeEngineIdentity{Endpoint: x.b.Endpoint, ID: x.b.EngineID}, Current: id, Generations: []port.HomeGeneration{g}}}
	x.engine.Homes[g.Name] = port.HomeObservation{Name: g.Name, Created: g.Created, Driver: "local", Scope: "local", Token: id, Key: key, ExactLabels: true, EmptyOptions: true}
	x.engine.Roots[g.Name] = f.HomeRoot{UID: x.b.UID, GID: x.b.GID, Mode: 0700, Directory: true, Marker: "retained"}
	return g
}
func (x homeFixture) run(t *testing.T) port.RunResult {
	t.Helper()
	return x.runner.Run(t.Context(), x.frozen, []string{"probe"}, port.Streams{})
}

type homeConsent struct {
	yes    bool
	before func()
}

func (c homeConsent) Review(context.Context, port.HomeEntry, string) bool {
	if c.before != nil {
		c.before()
	}
	return c.yes
}
func homeOutcome(r port.HomeResult) port.Outcome {
	if len(r.Failures) == 0 {
		return ""
	}
	return r.Failures[0].Outcome
}

func TestHomeRunLifecycleAndFaults(t *testing.T) {
	for _, fault := range []string{"fresh", "retained", "no service", "store unavailable", "store unsafe", "intent publication", "owned publication", "initialized publication", "create", "inspect", "initialize", "missing", "foreign", "mode", "claim", "pending", "deleted", "entropy"} {
		t.Run(fault, func(t *testing.T) {
			x := domainHomeFixture()
			want := port.HomeUnavailable
			switch fault {
			case "fresh":
				want = ""
			case "retained":
				x.seed()
				want = ""
			case "no service":
				x.runner.Homes = nil
			case "store unavailable":
				x.store.Failure = port.Incomplete
			case "store unsafe":
				x.store.Failure = port.Unsafe
				want = port.HomeRefused
			case "entropy":
				x.runner.Homes.Entropy = f.Entropy{Value: "invalid"}
			case "intent publication", "owned publication", "initialized publication":
				x.store.BeforeSave = func(s port.HomeScope) port.Outcome {
					g := s.Generations[0]
					if fault == "intent publication" && g.Stage == "intent" || fault == "owned publication" && g.Stage == "retained" || fault == "initialized publication" && g.Initialized {
						return port.IntentWrite
					}
					return ""
				}
			case "create", "inspect", "initialize":
				x.engine.HomeFault = fault
			default:
				g := x.seed()
				key := x.store.Scope(x.b)
				s := x.store.Scopes[key]
				want = port.HomeRefused
				switch fault {
				case "missing":
					delete(x.engine.Homes, g.Name)
				case "foreign":
					o := x.engine.Homes[g.Name]
					o.Created = "replacement"
					x.engine.Homes[g.Name] = o
				case "mode":
					r := x.engine.Roots[g.Name]
					r.Mode = 0777
					x.engine.Roots[g.Name] = r
				case "claim":
					s.Generations[0].Claims = []port.HomeClaim{{Session: strings.Repeat("b", 64)}}
					want = port.HomeUnavailable
				case "pending":
					s.Generations[0].Stage = "delete-pending"
					want = port.HomeUnavailable
				case "deleted":
					s.Generations[0].Stage = "deleted"
				}
				x.store.Scopes[key] = s
			}
			result := x.run(t)
			if result.Outcome != want || result.Started != (want == "") {
				t.Fatalf("result %+v, want %s", result, want)
			}
			if slices.Contains(x.engine.HomeEvents, "remove") {
				t.Fatal("session cleanup removed home")
			}
			if want == "" {
				if !result.Exited || result.Exit != 17 || result.Cleanup != port.Complete {
					t.Fatal(result)
				}
				state := (Recovery{Registry: x.registry, Engine: x.engine, Homes: &x.h}).Status(t.Context())
				if len(state.Failures) != 0 || state.Entries[0].Home.State != "present" || state.Entries[0].Cleanup != "complete" {
					t.Fatal(state)
				}
				if homes := x.h.ListHomes(t.Context()); len(homes.Homes) != 1 || homeOutcome(homes) != "" {
					t.Fatal(homes)
				}
			}
		})
	}
}

func TestHomeRetirementAndExactRetry(t *testing.T) {
	for _, action := range []string{"reset", "delete"} {
		for _, fault := range []string{"present", "absent", "refuse consent", "claim", "changed review", "foreign volume", "engine unavailable", "wrong engine", "intent write", "completion write", "remove", "entropy", "pending other action"} {
			t.Run(action+"/"+fault, func(t *testing.T) {
				x := domainHomeFixture()
				g := x.seed()
				key := x.store.Scope(x.b)
				consent := homeConsent{yes: true}
				want := port.Outcome("")
				switch fault {
				case "absent":
					delete(x.engine.Homes, g.Name)
				case "refuse consent":
					consent.yes = false
					want = port.ConsentRequired
				case "claim":
					s := x.store.Scopes[key]
					s.Generations[0].Claims = []port.HomeClaim{{Session: strings.Repeat("b", 64), StateRoot: "/original", Intent: "original"}}
					x.store.Scopes[key] = s
					want = port.HomeInUse
				case "changed review":
					consent.before = func() { s := x.store.Scopes[key]; s.Project = strings.Repeat("b", 64); x.store.Scopes[key] = s }
					want = port.Refused
				case "foreign volume":
					o := x.engine.Homes[g.Name]
					o.Token = "other"
					x.engine.Homes[g.Name] = o
					want = port.Refused
				case "engine unavailable":
					x.engine.Unavailable = true
					want = port.Incomplete
				case "wrong engine":
					x.engine.DaemonID = "other"
					want = port.Environment
				case "intent write":
					x.store.BeforeSave = func(port.HomeScope) port.Outcome { return port.IntentWrite }
					want = port.IntentWrite
				case "completion write":
					x.store.BeforeSave = func(s port.HomeScope) port.Outcome {
						if !strings.HasSuffix(s.Generations[0].Stage, "-pending") {
							return port.IntentWrite
						}
						return ""
					}
					want = port.IntentWrite
				case "remove":
					x.engine.HomeFault = "remove"
					want = port.Incomplete
				case "entropy":
					if action == "reset" {
						x.h.Entropy = f.Entropy{Value: g.ID}
						want = port.Internal
					}
				case "pending other action":
					s := x.store.Scopes[key]
					s.Generations[0].Stage = "reset-pending"
					if action == "reset" {
						s.Generations[0].Stage = "delete-pending"
					}
					x.store.Scopes[key] = s
					want = port.Refused
				}
				result := x.h.Retire(t.Context(), g.ID, action, consent)
				if homeOutcome(result) != want {
					t.Fatalf("%+v, want %s", result, want)
				}
				if want == "" {
					if result.Homes[0].State != "absent" || len(x.engine.Homes) != 0 {
						t.Fatal(result)
					}
					before := len(x.engine.HomeEvents)
					retry := x.h.Retire(t.Context(), g.ID, action, nil)
					if homeOutcome(retry) != "" || !reflect.DeepEqual(retry.Homes, result.Homes) || slices.Contains(x.engine.HomeEvents[before:], "remove") {
						t.Fatal("retry changed retired generation", retry)
					}
				}
			})
		}
	}
}

func TestHomeDiscoveryAndRefusedLookup(t *testing.T) {
	x := domainHomeFixture()
	if r := x.h.ListHomes(t.Context()); homeOutcome(r) != "" || len(r.Homes) != 0 {
		t.Fatal(r)
	}
	for _, input := range []struct {
		id, action string
		want       port.Outcome
	}{{"bad", "reset", port.Usage}, {strings.Repeat("a", 64), "bad", port.Usage}, {strings.Repeat("a", 64), "reset", port.Refused}} {
		if r := x.h.Retire(t.Context(), input.id, input.action, nil); homeOutcome(r) != input.want {
			t.Fatal(r)
		}
	}
	x.store.Failure = port.Unsafe
	if r := x.h.ListHomes(t.Context()); homeOutcome(r) != port.Unsafe || len(r.Homes) != 0 {
		t.Fatal(r)
	}
	if r := x.h.Retire(t.Context(), strings.Repeat("a", 64), "reset", nil); homeOutcome(r) != port.Unsafe {
		t.Fatal(r)
	}
}

func TestHomeRetirementDoesNotOutliveScopeLockBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		x := domainHomeFixture()
		g := x.seed()
		lock, failure := x.store.Open(t.Context(), x.store.Scope(x.b), false)
		if failure != "" {
			t.Fatal(failure)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		start := time.Now()
		result := x.h.Retire(ctx, g.ID, "reset", homeConsent{yes: true})
		if homeOutcome(result) != port.HomeUnavailable || time.Since(start) != time.Second {
			t.Fatal(result, time.Since(start))
		}
		lock.Close()
		synctest.Wait()
		if len(x.engine.Homes) != 1 || slices.Contains(x.engine.HomeEvents, "remove") {
			t.Fatal("expired retirement mutated storage")
		}
	})
}

func TestHomeRecoveryCannotClaimCompletionWithoutHomeService(t *testing.T) {
	x := domainHomeFixture()
	result := x.run(t)
	if result.Outcome != "" {
		t.Fatal(result)
	}
	recovery := Recovery{Registry: x.registry, Engine: x.engine}
	for _, got := range []port.RecoveryResult{recovery.Status(t.Context()), recovery.Stop(t.Context(), result.SessionID)} {
		if len(got.Failures) != 1 || got.Failures[0].Outcome != port.Incomplete {
			t.Fatal(got)
		}
	}
}
func TestHomeChangedRootBeforeFinalStartRefuses(t *testing.T) {
	x := domainHomeFixture()
	g := x.seed()
	roots := 0
	x.engine.HomeBefore = func(event string) {
		if event == "root" {
			roots++
			if roots == 2 {
				r := x.engine.Roots[g.Name]
				r.Mode = 0777
				x.engine.Roots[g.Name] = r
			}
		}
	}
	r := x.run(t)
	if r.Outcome != port.HomeRefused || r.Started || roots != 2 {
		t.Fatal(r, roots)
	}
}

func TestHomeUnallocatedSuccessorCannotBeRetired(t *testing.T) {
	x := domainHomeFixture()
	g := x.seed()
	s := x.store.Scopes[x.store.Scope(x.b)]
	s.Generations[0].Stage = "awaiting-allocation"
	s.Generations[0].Created = ""
	s.Generations[0].Initialized = false
	x.store.Scopes[s.Scope] = s
	delete(x.engine.Homes, g.Name)
	result := x.h.Retire(t.Context(), g.ID, "delete", homeConsent{yes: true})
	if homeOutcome(result) != port.Incomplete {
		t.Fatal(result)
	}
}
