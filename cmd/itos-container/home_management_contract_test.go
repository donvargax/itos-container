package main

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func homeManagementControl(t *testing.T, action string) {
	t.Helper()
	f := newHomeLocalFixture(t, "1000", "1000")
	g := f.seedHome(t)
	f.consent = homeLocalConsent{yes: true}
	e := f.retire(t, g.ID, action, 0)
	if e.State != "absent" || len(f.engine.Homes) != 0 {
		t.Fatal("consenting home control did not retire storage", e)
	}
}
func homeCatalogDiscovery(t *testing.T) {
	f := newHomeLocalFixture(t, "1000", "1000")
	g := f.seedHome(t)
	s := f.scope()
	// Independent obsolete GID and original daemon scope, with a deleted
	// generation. The fixture never asks the product to create setup state.
	old := s
	old.GID = "3001"
	old.Engine.ID = "original-daemon"
	b := old.Binding()
	b.Frozen.Project = old.Project
	old.Scope = homeFixtureCodec{}.Scope(b)
	old.Current = strings.Repeat("e", 64)
	og := sp.HomeGeneration{ID: old.Current, Name: "itos-container-home-" + old.Current, Created: "old-created", Stage: "deleted", Initialized: true, Claims: []sp.HomeClaim{}}
	old.Generations = []sp.HomeGeneration{og}
	f.putScope(old)
	before := homeFixtureCodec{}.EncodeHome(old)
	code, out, d := f.command(t, "status", "--homes", "--json")
	var report struct {
		Schema int
		OK     bool
		Homes  []sp.HomeEntry
	}
	if json.Unmarshal([]byte(out), &report) != nil || code != 3 || len(report.Homes) != 2 || report.Homes[0].ID != g.ID || report.Homes[1].ID != og.ID || report.Homes[1].State != "unknown" || !strings.Contains(d, "recovery.environment") {
		t.Fatal("home catalog discovery", code, out, d)
	}
	if !bytes.Equal(before, homeFixtureCodec{}.EncodeHome(f.homes.Scopes[old.Scope])) {
		t.Fatal("discovery changed obsolete state")
	}
	f.noHomeMutation(t, 0)
	empty := newHomeLocalFixture(t, "1000", "1000")
	code, out, d = empty.command(t, "status", "--homes", "--json")
	if code != 0 || !strings.Contains(out, "\"homes\":[]") || len(empty.homes.Scopes) != 0 || len(empty.engine.Events) != 0 {
		t.Fatal("empty catalog created state", code, out, d)
	}
	code, out, d = f.command(t, "status", "--homes")
	if code != 3 || !strings.Contains(out, g.ID) || strings.Contains(out, "original marker") {
		t.Fatal("plain home catalog", code, out, d)
	}
}
func homeConsentContract(t *testing.T, response string) {
	for _, action := range []string{"reset", "delete"} {
		for _, jsonMode := range []bool{false, true} {
			for _, answer := range []string{response, "terminal yes"} {
				f := newHomeLocalFixture(t, "1000", "1000")
				g := f.seedHome(t)
				before := homeScopeBytes(f)
				f.consent = homeConsentFor(t, f, g, answer)
				args := []string{"stop", "--home", g.ID, "--" + action}
				if jsonMode {
					args = append(args, "--json")
				}
				code, out, d := f.command(t, args...)
				if answer == "terminal yes" {
					if code != 0 || len(f.engine.Homes) != 0 {
						t.Fatal("terminal consent did not retire", code, out, d)
					}
				} else {
					if code != 1 || !strings.Contains(d, "recovery.consent-required") {
						t.Fatal(response, code, out, d)
					}
					homeUnchanged(t, f, before)
					f.noHomeMutation(t, 0)
				}
				if jsonMode {
					var value map[string]any
					if json.Unmarshal([]byte(out), &value) != nil || value["schema"] != float64(1) {
						t.Fatal("prompt contaminated stdout", out)
					}
				}
			}
		}
	}
}
func homeResetContract(t *testing.T, condition string) {
	homeManagementControl(t, "reset")
	f := newHomeLocalFixture(t, "1000", "1000")
	g := f.seedHome(t)
	switch condition {
	case "missing after external removal":
		delete(f.engine.Homes, g.Name)
	case "deleted by prior consent":
		delete(f.engine.Homes, g.Name)
		s := f.scope()
		s.Generations[0].Stage = "deleted"
		f.putScope(s)
	case "owned with unsafe worker root mode":
		r := f.engine.Roots[g.Name]
		r.Mode = 0777
		f.engine.Roots[g.Name] = r
	case "present with a retained marker":
	default:
		t.Fatal(condition)
	}
	want, rule := 1, "run.home-refused"
	if condition == "present with a retained marker" {
		want, rule = 0, ""
	}
	homeRequireRun(t, f, want, rule)
	f.consent = homeLocalConsent{yes: true}
	before := len(f.engine.Events)
	e := f.retire(t, g.ID, "reset", 0)
	for _, event := range f.engine.Events[before:] {
		if event == "create" || event == "start" || event == "signal" {
			t.Fatal("reset changed session", event)
		}
	}
	if e.Successor == nil || len(f.engine.Homes) != 0 {
		t.Fatal("reset allocated successor", e)
	}
	homeRequireRun(t, f, 0, "")
	name := "itos-container-home-" + *e.Successor
	if f.engine.Roots[name].Marker != "" {
		t.Fatal("successor inherited old data")
	}
	r := f.engine.Roots[name]
	r.Marker = "successor marker"
	f.engine.Roots[name] = r
	f.consent = homeLocalConsent{}
	again := f.retire(t, g.ID, "reset", 0)
	if again.Successor == nil || *again.Successor != *e.Successor || f.engine.Roots[name].Marker != r.Marker {
		t.Fatal("old-ID reset changed successor")
	}
}
func homeObsoleteDeletion(t *testing.T, identity string) {
	homeManagementControl(t, "delete")
	f := newHomeLocalFixture(t, "1000", "1000")
	g := f.seedHome(t)
	original := f.scope()
	other := original
	switch identity {
	case "host GID":
		other.GID = "3001"
	case "checkout identity":
		other.Project = strings.Repeat("e", 64)
	case "local daemon":
		other.Engine.ID = "other"
	default:
		t.Fatal(identity)
	}
	b := other.Binding()
	b.Frozen.Project = other.Project
	other.Scope = homeFixtureCodec{}.Scope(b)
	other.Current = strings.Repeat("f", 64)
	other.Generations = []sp.HomeGeneration{{ID: other.Current, Name: "itos-container-home-" + other.Current, Created: "other-created", Stage: "retained", Initialized: true, Claims: []sp.HomeClaim{}}}
	f.putScope(other)
	og := other.Generations[0]
	f.engine.Homes[og.Name] = homeObservation(og, other.Scope)
	r := f.engine.Roots[g.Name]
	r.Marker = "other retained marker"
	f.engine.Roots[og.Name] = r
	f.consent = homeLocalConsent{yes: true}
	f.retire(t, g.ID, "delete", 0)
	if f.engine.Roots[og.Name].Marker != r.Marker || len(f.engine.Homes) != 1 || f.scope().Generations[0].Stage != "deleted" {
		t.Fatal("deletion crossed scope")
	}
	before := len(f.engine.HomeEvents)
	f.retire(t, g.ID, "delete", 0)
	f.noHomeMutation(t, before)
	f.omitHome(t)
	homeRequireRun(t, f, 0, "")
	f.enableHome(t)
	homeRequireRun(t, f, 1, "run.home-refused")
	f.retire(t, g.ID, "reset", 0)
	homeRequireRun(t, f, 0, "")
}
func homeRetirementSubstitution(t *testing.T, defect string) {
	for _, action := range []string{"reset", "delete"} {
		homeManagementControl(t, action)
		for _, boundary := range []string{"review", "remove"} {
			f := newHomeLocalFixture(t, "1000", "1000")
			g := f.seedHome(t)
			expected := 1
			mutate := func() {
				s := f.scope()
				o := f.engine.Homes[g.Name]
				switch defect {
				case "another host UID in the record":
					s.UID = "2001"
				case "replaced protected generation":
					s.Generations[0].Created = "replaced"
				case "symlinked home record":
					f.homes.Failure = sp.Unsafe
				case "missing original ownership tuple":
					f.homes.Failure = sp.Invalid
				case "same name different creation time":
					o.Created = "replaced"
				case "wrong scope or token labels":
					o.Token = strings.Repeat("f", 64)
				case "changed driver or options":
					o.Driver = "nfs"
				case "replaced selected socket or daemon":
					f.engine.Replaced = true
				case "unavailable original daemon":
					f.engine.HomeFault = "inspect"
				default:
					t.Fatal(defect)
				}
				f.putScope(s)
				f.engine.Homes[g.Name] = o
			}
			if defect == "missing original ownership tuple" {
				expected = 2
			}
			if defect == "unavailable original daemon" {
				expected = 75
			}
			if boundary == "review" {
				f.consent = homeLocalConsent{yes: true, before: func(sp.HomeEntry, string) { mutate() }}
			} else {
				f.consent = homeLocalConsent{yes: true}
				fired := false
				f.engine.HomeBefore = func(event string) {
					if event == "inspect" && !fired && strings.HasSuffix(f.scope().Generations[0].Stage, "-pending") {
						fired = true
						mutate()
					}
				}
			}
			f.retire(t, g.ID, action, expected)
			wantRule := "recovery.not-owned"
			switch defect {
			case "symlinked home record":
				wantRule = "recovery.unsafe-state"
			case "missing original ownership tuple":
				wantRule = "recovery.invalid-state"
			case "unavailable original daemon":
				wantRule = "recovery.temporary"
			}
			if f.lastRule != wantRule {
				t.Fatal("wrong ownership diagnosis", defect, boundary, f.lastRule, wantRule)
			}
			for _, event := range f.engine.HomeEvents {
				if event == "remove" {
					t.Fatal("substitution was removed", defect, boundary)
				}
			}
		}
	}
}
func homeClaimFence(t *testing.T, reference string) {
	for _, action := range []string{"reset", "delete"} {
		homeManagementControl(t, action)
		f := newHomeLocalFixture(t, "1000", "1000")
		id, worker, g := f.seedHomeSession(t, reference)
		manager := homeOtherState(t, f)
		manager.consent = homeLocalConsent{yes: true}
		var entry sp.Intent
		if reference == "running with a held session operation lock" {
			// The claiming session's operation holds its intents entry lock
			// for the operation's whole lifetime, as a live run, stop or
			// status of that project does. Claim evidence must be judged
			// without that lock: the consented action names the in-use home
			// within the operation budget, never the lock as a temporary
			// failure, and the home stays intact.
			opened, failure := f.m.registry.Store.Open(context.Background(), sp.Reference(f.m.c.Project, strings.Repeat("b", 63)+"1"))
			if failure != "" || opened == nil {
				t.Fatal("held session entry", failure)
			}
			entry = opened
		}
		before := len(f.engine.Events)
		start := time.Now()
		e := manager.retire(t, g.ID, action, 1)
		if manager.lastRule != "recovery.home-in-use" {
			t.Fatal("known claim lost its diagnosis", manager.lastRule)
		}
		if entry != nil && time.Since(start) > 10*time.Second {
			t.Fatal("claim judgment waited out the operation budget")
		}
		if entry != nil {
			entry.Close()
		}
		wantWorkers := 1
		if reference == "uncertain worker create" || reference == "paused launch before worker create" {
			wantWorkers = 0
		}
		if !slices.Equal(e.References, []string{id}) || len(f.engine.Resources) != wantWorkers {
			t.Fatal("home claim not retained", e)
		}
		for _, event := range f.engine.Events[before:] {
			if event == "signal" || event == "remove" {
				t.Fatal("management stopped worker")
			}
		}
		if _, ok := f.engine.Resources[worker]; !ok && wantWorkers != 0 {
			t.Fatal("worker removed")
		}
		if reference == "uncertain worker create" {
			// The delayed engine create becomes observable before stop. Name-only
			// absence above must not have been treated as release evidence.
			token := strings.Repeat("b", 63) + "1"
			i := f.m.b.Intent(token)
			f.engine.Resources[worker] = sp.Observation{ID: worker, Name: i.Name, ImageID: i.ImageID, Token: token, Project: i.Project, Fingerprint: i.Fingerprint, StatePresent: true, Status: "created", HomeName: g.Name, HomeExact: true, AnchorName: "itos-container-anchor-" + token, AnchorExact: true}
		}
		code, out, d := f.command(t, "stop", id, "--json")
		if code != 0 || len(f.scope().Generations[0].Claims) != 0 {
			t.Fatal("session did not release claim", code, out, d)
		}
		manager.retire(t, g.ID, action, 0)
		// A launch claiming while the terminal waits invalidates that review.
		f = newHomeLocalFixture(t, "1000", "1000")
		g = f.seedHome(t)
		f.consent = homeLocalConsent{yes: true, before: func(sp.HomeEntry, string) { f.seedHomeSession(t, reference) }}
		f.retire(t, g.ID, action, 1)
		if f.lastRule != "recovery.home-in-use" {
			t.Fatal("claim during consent lost its diagnosis", f.lastRule)
		}
		f.noHomeMutation(t, 0)
		// A durable pending action fences all later launch/mount attempts.
		f = newHomeLocalFixture(t, "1000", "1000")
		f.seedHome(t)
		s := f.scope()
		s.Generations[0].Stage = action + "-pending"
		if action == "reset" {
			s.Generations[0].Successor = strings.Repeat("f", 64)
		}
		f.putScope(s)
		homeRequireRun(t, f, 75, "run.home-unavailable")
		f.noHomeMutation(t, 0)
	}
}
func homeRetirementFault(t *testing.T, fault string) {
	for _, action := range []string{"reset", "delete"} {
		homeManagementControl(t, action)
		run := func(t *testing.T) {
			f := newHomeLocalFixture(t, "1000", "1000")
			g := f.seedHome(t)
			f.consent = homeLocalConsent{yes: true}
			var cancel context.CancelFunc
			release := make(chan struct{})
			switch fault {
			case "completion publication fsync fails":
				f.homes.BeforeSave = func(s sp.HomeScope) sp.Outcome {
					if s.Generations[0].Stage == "reset" || s.Generations[0].Stage == "deleted" {
						return sp.IntentWrite
					}
					return ""
				}
			case "controller dies after durable action intent":
				f.homes.BeforeSave = func(s sp.HomeScope) sp.Outcome {
					if strings.HasSuffix(s.Generations[0].Stage, "-pending") {
						f.engine.HomeFault = "inspect"
					}
					return ""
				}
			case "controller dies after removal", "removal reply lost and absence inspection unavailable":
				f.engine.HomeBefore = func(event string) {
					if event == "remove" {
						f.engine.HomeFault = "inspect"
					}
				}
			case "engine reports volume in use":
				f.engine.HomeFault = "remove"
			case "engine removal consumes remaining budget":
				f.ctx, cancel = context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				f.engine.HomeBefore = func(event string) {
					if event == "remove" {
						<-release
					}
				}
			default:
				t.Fatal(fault)
			}
			e := f.retire(t, g.ID, action, 75)
			if e.Disposition != action+"-pending" || !slices.Equal(e.Retry, []string{"itos", "container", "stop", "--home", g.ID, "--" + action}) {
				t.Fatal("missing pending action", e)
			}
			if cancel != nil {
				close(release)
				if f.disk == nil {
					synctest.Wait()
				} else {
					// The canceled HTTP reply can still be leaving the recorder.
					// Join it before changing the next request's fixture state.
					if slices.Contains(f.sdk.workerEvents(), "start") {
						t.Fatal("expired retirement started a worker")
					}
				}
			}
			f.ctx = nil
			f.homes.BeforeSave = nil
			f.engine.HomeBefore = nil
			f.engine.HomeFault = ""
			code, out, d := f.command(t, "status", "--homes", "--json")
			if code != 0 || !strings.Contains(out, action+"-pending") || !strings.Contains(out, "--"+action) {
				t.Fatal("fresh status forgot action", code, out, d)
			}
			homeRequireRun(t, f, 75, "run.home-unavailable")
			e = f.retire(t, g.ID, action, 0)
			if e.State != "absent" {
				t.Fatal("retry did not complete", e)
			}
			before := len(f.engine.HomeEvents)
			f.retire(t, g.ID, action, 0)
			f.noHomeMutation(t, before)
			o := homeObservation(g, f.scope().Scope)
			o.Created = "replacement"
			f.engine.Homes[g.Name] = o
			f.retire(t, g.ID, action, 1)
			f.noHomeMutation(t, before)
		}
		if fault == "engine removal consumes remaining budget" {
			homeContractClock(t, run)
		} else {
			run(t)
		}
	}
}
func homeInvalidAction(t *testing.T, condition string) {
	homeManagementControl(t, "reset")
	run := func(t *testing.T) {
		f := newHomeLocalFixture(t, "1000", "1000")
		g := f.seedHome(t)
		before := homeScopeBytes(f)
		f.consent = homeLocalConsent{yes: true}
		args := []string{"stop", "--home", g.ID, "--reset", "--json"}
		want := 2
		var lock sp.HomeTransaction
		switch condition {
		case "truncated home ID":
			args[2] = g.ID[:12]
		case "both reset and delete":
			args = append(args, "--delete")
		case "home ID and session ID":
			args = append(args, strings.Repeat("b", 64))
		case "home ID without action":
			args = []string{"stop", "--home", g.ID, "--json"}
		case "unattended yes override":
			args = append(args, "--yes")
		case "action intent fsync fails":
			want = 75
			f.homes.BeforeSave = func(sp.HomeScope) sp.Outcome { return sp.IntentWrite }
		case "scope lock exhausts the budget":
			want = 75
			var cancel context.CancelFunc
			f.ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			lock, _ = f.service().Store.Open(context.Background(), f.scope().Scope, false)
			defer lock.Close()
		case "pending allocation with name-only 404":
			want = 75
			s := f.scope()
			s.Generations[0].Created = ""
			s.Generations[0].Stage = "intent"
			s.Generations[0].Initialized = false
			f.putScope(s)
			delete(f.engine.Homes, g.Name)
			before = homeScopeBytes(f)
		case "delayed create landed at intent":
			// The create reached the engine with the exact token and scope
			// labels and its reply was lost, so no Created is recorded. The
			// owned observation must not read as a foreign volume: management
			// reports the pending allocation as temporary, exactly as the
			// claiming session's stop resolves it.
			want = 75
			s := f.scope()
			s.Generations[0].Created = ""
			s.Generations[0].Stage = "intent"
			s.Generations[0].Initialized = false
			f.putScope(s)
			before = homeScopeBytes(f)
		case "claim state root unavailable":
			want = 75
			f.homes.ClaimsFailure = sp.Incomplete
		case "unsafe session claim":
			want = 1
			f.homes.ClaimsFailure = sp.Unsafe
		default:
			t.Fatal(condition)
		}
		code, out, d := f.command(t, args...)
		if code != want {
			t.Fatal("invalid home action", condition, code, out, d, want)
		}
		homeUnchanged(t, f, before)
		f.noHomeMutation(t, 0)
		if want == 2 && condition != "missing original ownership tuple" && len(f.engine.Events) != 0 {
			t.Fatal("usage read engine", f.engine.Events)
		}
	}
	if condition == "scope lock exhausts the budget" {
		homeContractClock(t, run)
	} else {
		run(t)
	}
}
