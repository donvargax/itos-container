package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func homeRequireRun(t *testing.T, f *homeLocalFixture, want int, rule string) {
	t.Helper()
	code, _, diagnostic := f.command(t, "run", "--", "probe")
	if code != want || rule != "" && !strings.Contains(diagnostic, rule) {
		t.Fatalf("home run: exit %d, %s; want %d %s", code, diagnostic, want, rule)
	}
}
func homePositive(t *testing.T) {
	t.Helper()
	homeReuseContract(t, "1000", "1000")
}
func (f *homeLocalFixture) scope() sp.HomeScope {
	return f.homes.Scopes[homeFixtureCodec{}.Scope(f.m.b)]
}
func (f *homeLocalFixture) putScope(s sp.HomeScope) { f.homes.Scopes[s.Scope] = s }
func (f *homeLocalFixture) noHomeMutation(t *testing.T, from int) {
	t.Helper()
	for _, event := range f.engine.HomeEvents[from:] {
		if event == "remove" || event == "allocate" || event == "initialize" {
			t.Fatal("unexpected home mutation", event)
		}
	}
}
func homeApprovalChanges(t *testing.T) {
	homePositive(t)
	for _, enabled := range []bool{true, false} {
		f := newHomeLocalFixture(t, "1000", "1000")
		g := f.seedHome(t)
		if !enabled {
			f.omitHome(t)
		}
		f.approvalStatus = "changed"
		homeRequireRun(t, f, 1, "run.approval-changed")
		if len(f.engine.HomeEvents) != 0 || len(f.engine.Events) != 0 {
			t.Fatal("approval mismatch reached engine")
		}
		f.approvalStatus = ""
		homeRequireRun(t, f, 0, "")
		if f.engine.Roots[g.Name].Marker != "original" || len(f.engine.Homes) != 1 {
			t.Fatal("toggle changed retained data")
		}
		if !enabled {
			f.enableHome(t)
			homeRequireRun(t, f, 0, "")
		}
	}
}
func (f *homeLocalFixture) omitHome(t *testing.T) {
	t.Helper()
	var v map[string]any
	_ = json.Unmarshal([]byte(f.m.c.Canonical), &v)
	delete(v, "home")
	v["profile"] = "offline-v1"
	f.settings(t, v, false)
}
func (f *homeLocalFixture) enableHome(t *testing.T) {
	t.Helper()
	var v map[string]any
	_ = json.Unmarshal([]byte(f.m.c.Canonical), &v)
	v["profile"] = "offline-home-v1"
	v["home"] = map[string]string{"storage": "volume", "target": "/home/itos", "mode": "0700", "lifetime": "project"}
	f.settings(t, v, true)
}
func homeFrozenSelection(t *testing.T) {
	homePositive(t)
	f := newHomeLocalFixture(t, "1000", "1000")
	original := f.m.b.Frozen
	f.beforeRun = func(ap.Durable) { f.omitHome(t); f.beforeRun = nil }
	homeRequireRun(t, f, 0, "")
	if len(f.engine.Homes) != 1 || len(f.scope().Generations) != 1 {
		t.Fatal("selected home disappeared after request change")
	}
	if string(f.m.storage.Directories["snapshots/"+f.m.c.Project].Files["1.json"]) == "" || original.Settings == f.m.c.Canonical {
		t.Fatal("frozen input fixture did not change requests")
	}
	before := len(f.engine.HomeEvents)
	homeRequireRun(t, f, 0, "")
	f.noHomeMutation(t, before)
}
func homeStartupFault(t *testing.T, fault string) {
	homePositive(t)
	f := newHomeLocalFixture(t, "1000", "1000")
	once := false
	f.homes.BeforeSave = func(s sp.HomeScope) sp.Outcome {
		g := s.Generations[0]
		match := fault == "intent publication fails" && g.Stage == "intent" ||
			fault == "owned home publication fails" && g.Stage == "retained" && g.Created != "" ||
			fault == "initialized home publication fails" && g.Initialized
		if match && !once {
			once = true
			return sp.IntentWrite
		}
		return ""
	}
	switch fault {
	case "volume create reply is lost":
		f.engine.HomeFault = "create"
	case "root initialization fails":
		f.engine.HomeFault = "initialize"
	case "home inspection is unavailable":
		f.engine.HomeFault = "inspect"
	}
	homeRequireRun(t, f, 75, "run.home-unavailable")
	for _, event := range f.engine.Events {
		if event == "start" {
			t.Fatal("faulted home started")
		}
	}
	f.engine.HomeFault = ""
	f.homes.BeforeSave = nil
	ids, _ := f.m.registry.List(context.Background())
	for _, id := range ids {
		code, _, d := f.command(t, "stop", id, "--json")
		if code != 0 && code != 75 {
			t.Fatal("home recovery", code, d)
		}
	}
	creates := 0
	for _, event := range f.engine.HomeEvents {
		if event == "allocate" {
			creates++
		}
		if event == "remove" {
			t.Fatal("session cleanup deleted home")
		}
	}
	if creates > 1 {
		t.Fatal("uncertain allocation retried")
	}
}
func homeReuseDefect(t *testing.T, defect string) {
	homePositive(t)
	f := newHomeLocalFixture(t, "1000", "1000")
	g := f.seedHome(t)
	s := f.scope()
	o := f.engine.Homes[g.Name]
	root := f.engine.Roots[g.Name]
	switch defect {
	case "occupied freshly allocated random name":
		s.Generations[0].Stage = "awaiting-allocation"
		s.Generations[0].Created = ""
		s.Generations[0].Initialized = false
	case "recorded volume is absent":
		delete(f.engine.Homes, g.Name)
	case "same name with different creation time":
		o.Created = "replacement"
	case "another checkout scope in labels":
		o.Key = strings.Repeat("e", 64)
	case "another daemon in protected ownership":
		s.Engine.ID = "other"
	case "nonlocal driver":
		o.Driver = "nfs"
	case "nonempty driver options":
		o.EmptyOptions = false
	case "root owner differs from approved UID GID":
		root.GID = "42"
	case "root mode is not 0700":
		root.Mode = 0777
	case "home reference belongs to another project":
		s.Project = strings.Repeat("e", 64)
	default:
		t.Fatal("unknown home defect", defect)
	}
	if defect != "recorded volume is absent" {
		f.engine.Homes[g.Name] = o
	}
	f.engine.Roots[g.Name] = root
	f.putScope(s)
	homeRequireRun(t, f, 1, "run.home-refused")
	f.noHomeMutation(t, 0)
	for _, event := range f.engine.Events {
		if event == "start" {
			t.Fatal("unowned home started")
		}
	}
}
func homeBoundaryDefect(t *testing.T, defect string) {
	homePositive(t)
	f := newHomeLocalFixture(t, "1000", "1000")
	f.boundary = defect
	f.engine.Boundary = sp.IsolationUnverified
	homeRequireRun(t, f, 1, "run.isolation-unverified")
	if len(f.engine.Homes) != 1 || len(f.engine.Resources) != 0 {
		t.Fatal("bad boundary cleanup", f.engine.Homes, f.engine.Resources)
	}
	for _, event := range f.engine.Events {
		if event == "start" {
			t.Fatal("bad boundary started")
		}
	}
	for _, event := range f.engine.HomeEvents {
		if event == "remove" {
			t.Fatal("boundary refusal removed home")
		}
	}
}
func (f *homeLocalFixture) seedHomeSession(t *testing.T, state string) (string, string, sp.HomeGeneration) {
	t.Helper()
	g := f.seedHome(t)
	id, worker := f.m.seed("b")
	token := strings.Repeat("b", 63) + "1"
	i := f.m.b.Intent(token)
	i.Home = g.Reference(f.scope().Scope)
	i.Stage, i.ContainerID = "owned", worker
	f.m.storage.Directories["intents/"+f.m.c.Project].Files[token+".json"] = f.m.registry.Store.Codec.Encode(i)
	s := f.scope()
	s.Generations[0].Claims = []sp.HomeClaim{{Session: id, StateRoot: "/fixture/state/itos-container", Intent: sp.Reference(i.Project, token)}}
	f.putScope(s)
	o := f.engine.Resources[worker]
	o.HomeName, o.HomeExact = g.Name, true
	switch state {
	case "exited with observed code 7", "exited worker not yet removed":
		o.Status, o.Started = "exited", true
		r := sp.PublicRecord{Schema: 1, ID: id, Project: i.Project, Intent: sp.Reference(i.Project, token), State: "exited"}
		value := 7
		r.WorkloadExit = &value
		f.m.files.Sessions[f.m.index().Project+"/"+id].Data = f.m.registry.Codec.EncodePublic(r)
	case "running after controller loss", "running with live controller", "running after controller death":
		o.Status, o.Running, o.Started, o.PID = "running", true, true, 123
	}
	f.engine.Resources[worker] = o
	return id, worker, g
}
func homeSessionRecovery(t *testing.T, state string) {
	homePositive(t)
	f := newHomeLocalFixture(t, "1000", "1000")
	id, _, g := f.seedHomeSession(t, state)
	f.omitHome(t)
	code, out, d := f.command(t, "status", "--json")
	if code != 0 || !strings.Contains(out, g.Name) || !strings.Contains(out, "present") {
		t.Fatal("original home status", code, out, d)
	}
	code, out, d = f.command(t, "stop", id, "--json")
	if code != 0 || !strings.Contains(out, "complete") || state == "exited with observed code 7" && !strings.Contains(out, "\"workload_exit\":7") {
		t.Fatal("original session stop", code, out, d)
	}
	if len(f.engine.Resources) != 0 || len(f.scope().Generations[0].Claims) != 0 || f.engine.Roots[g.Name].Marker != "original" {
		t.Fatal("cleanup failed or changed home")
	}
	before := len(f.engine.Events)
	code, out, d = f.command(t, "stop", id, "--json")
	if code != 0 {
		t.Fatal(code, out, d)
	}
	for _, event := range f.engine.Events[before:] {
		if event == "remove" || event == "signal" || event == "start" {
			t.Fatal("repeated stop mutated", event)
		}
	}
	f.noHomeMutation(t, 0)
	legacy := publicMemoryFixture(t)
	lid, _ := legacy.seed("d")
	publicMemoryCommand(t, legacy.service(), "stop", lid, 0)
}
func homeLateAllocation(t *testing.T) {
	homePositive(t)
	f := newHomeLocalFixture(t, "1000", "1000")
	id, worker, g := f.seedHomeSession(t, "unstarted")
	delete(f.engine.Resources, worker)
	delete(f.engine.Homes, g.Name)
	token := strings.Repeat("b", 63) + "1"
	i := f.m.b.Intent(token)
	i.Stage = "cancelled"
	i.Home = g.Reference(f.scope().Scope)
	i.Home.Created = ""
	f.m.storage.Directories["intents/"+f.m.c.Project].Files[token+".json"] = f.m.registry.Store.Codec.Encode(i)
	s := f.scope()
	s.Generations[0].Stage = "intent"
	s.Generations[0].Created = ""
	s.Generations[0].Initialized = false
	f.putScope(s)
	code, out, d := f.command(t, "stop", id, "--json")
	if code != 75 || !strings.Contains(out, id) {
		t.Fatal("name-only absence resolved allocation", code, out, d)
	}
	f.engine.Homes[g.Name] = homeObservation(g, f.scope().Scope)
	code, out, d = f.command(t, "stop", id, "--json")
	if code != 0 || f.scope().Generations[0].Created != g.Created || len(f.scope().Generations[0].Claims) != 0 {
		t.Fatal("delayed allocation not retained", code, out, d)
	}
	f.noHomeMutation(t, 0)
	o := homeObservation(g, f.scope().Scope)
	o.Created = "replacement"
	f.engine.Homes[g.Name] = o
	code, _, _ = f.command(t, "stop", id, "--json")
	if code != 1 {
		t.Fatal("replacement accepted", code)
	}
}
func homeOperationBudget(t *testing.T, operation string) {
	homePositive(t)
	synctest.Test(t, func(t *testing.T) {
		f := newHomeLocalFixture(t, "1000", "1000")
		var cancel context.CancelFunc
		f.ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		release := make(chan struct{})
		var lock sp.HomeTransaction
		switch operation {
		case "allocation lock during startup":
			var outcome sp.Outcome
			lock, outcome = f.homes.Open(context.Background(), homeFixtureCodec{}.Scope(f.m.b), true)
			if outcome != "" {
				t.Fatal(outcome)
			}
			defer lock.Close()
		case "volume create during startup":
			f.engine.HomeBefore = func(event string) {
				if event == "allocate" {
					<-release
				}
			}
		case "initialization during startup":
			f.engine.HomeBefore = func(event string) {
				if event == "initialize" {
					<-release
				}
			}
		case "home inspection during status", "reconciliation during stop":
			f.seedHomeSession(t, "unstarted")
			f.engine.HomeBefore = func(event string) {
				if event == "inspect" {
					<-release
				}
			}
		default:
			t.Fatal(operation)
		}
		start := time.Now()
		code := 0
		switch operation {
		case "home inspection during status":
			code, _, _ = f.command(t, "status", "--json")
		case "reconciliation during stop":
			code, _, _ = f.command(t, "stop", strings.Repeat("b", 64), "--json")
		default:
			code, _, _ = f.command(t, "run", "--", "probe")
		}
		if code != 75 || time.Since(start) > 31*time.Second {
			t.Fatal("home budget restarted", code, time.Since(start))
		}
		close(release)
		synctest.Wait()
		for _, event := range f.engine.Events {
			if event == "start" {
				t.Fatal("late home observation started worker")
			}
		}
	})
}
func homeSharedScope(t *testing.T) {
	homePositive(t)
	f := newHomeLocalFixture(t, "1000", "1000")
	id, _, g := f.seedHomeSession(t, "unstarted")
	homeRequireRun(t, f, 1, "run.checkout-overlap")
	if code, out, d := f.command(t, "stop", id, "--json"); code != 0 {
		t.Fatal(code, out, d)
	}
	homeRequireRun(t, f, 0, "")
	if len(f.engine.Homes) != 1 || f.engine.Roots[g.Name].Marker != "original" {
		t.Fatal("shared scope recreated home")
	}
	for _, event := range f.engine.HomeEvents {
		if event == "allocate" || event == "initialize" {
			t.Fatal("shared scope lost ownership", event)
		}
	}
}
func homeObservation(g sp.HomeGeneration, key string) sp.HomeObservation {
	return sp.HomeObservation{Name: g.Name, Created: g.Created, Driver: "local", Scope: "local", Token: g.ID, Key: key, ExactLabels: true, EmptyOptions: true}
}
func homeScopeBytes(f *homeLocalFixture) []byte { return homeFixtureCodec{}.EncodeHome(f.scope()) }
func homeUnchanged(t *testing.T, f *homeLocalFixture, before []byte) {
	t.Helper()
	if !bytes.Equal(before, homeScopeBytes(f)) {
		t.Fatal("protected home record changed")
	}
}
