package domain

import (
	"context"
	"slices"
	"strings"
	"testing"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type anchorFixture struct {
	intent     port.Intent
	connection port.RunConnection
	workspace  *f.Workspace
	store      *f.Store
	engine     *f.RunEngine
	sessionID  string
}

func newAnchorFixture(t *testing.T) anchorFixture {
	t.Helper()
	b, store, engine := f.Fixture()
	i, failure := store.Prepare(t.Context(), approval.Durable{Frozen: b.Frozen}, strings.Repeat("1", 64))
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(i.Close)
	e := &f.RunEngine{Engine: engine}
	c, failure := e.OpenRun(t.Context(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(c.Close)
	w := f.MountWorkspace(b)
	if failure := w.Pin(b); failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(w.Close)
	return anchorFixture{i, c, w, store, e, strings.Repeat("2", 64)}
}

func (a anchorFixture) prepare(t *testing.T) {
	t.Helper()
	if failure := prepareAnchor(t.Context(), a.intent, a.connection, a.workspace, a.sessionID); failure != "" {
		t.Fatal(failure)
	}
}

func TestPrepareAnchorStopsBeforeWorkerCreation(t *testing.T) {
	for _, name := range []string{"no anchor port", "already intent", "daemon changed", "inspection unavailable", "foreign volume", "owned publication failed"} {
		t.Run(name, func(t *testing.T) {
			a := newAnchorFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var workspace port.Workspace = a.workspace
			want := port.MountRefused
			switch name {
			case "no anchor port":
				workspace = struct{ port.Workspace }{a.workspace}
				want = port.MountUnavailable
			case "already intent":
				r := a.intent.Record()
				r.Stage = "intent"
				if failure := a.intent.Save(r); failure != "" {
					t.Fatal(failure)
				}
			case "daemon changed":
				a.store.AfterSave = func() { a.engine.Replaced = true }
				want = port.Refused
			case "inspection unavailable":
				a.engine.After = func(event string) {
					if event == "anchor inspect" {
						cancel()
					}
				}
				want = port.MountUncertain
			case "foreign volume":
				r := f.RunIntent(a.intent.Binding())
				resources := f.AnchorResources{Created: r.Anchor.Created}
				if failure := resources.AllocateAnchor(ctx, r); failure != "" {
					t.Fatal(failure)
				}
				o := resources.Resources[r.Anchor.Name]
				o.Source = "/proc/456/fd/9"
				resources.Resources[r.Anchor.Name] = o
				a.engine.Anchors = resources.Resources
			case "owned publication failed":
				a.engine.After = func(event string) {
					if event == "anchor inspect" {
						a.store.Fault = "reserved-record file fsync"
					}
				}
				want = port.MountWrite
			}
			if got := prepareAnchor(ctx, a.intent, a.connection, workspace, a.sessionID); got != want {
				t.Fatalf("preparation = %s, want %s", got, want)
			}
			if slices.Contains(a.engine.Events, "create") || slices.Contains(a.engine.Events, "start") || a.intent.Record().Anchor.Stage == "owned" {
				t.Fatal("failed preparation acquired worker or owned-anchor authority", a.engine.Events, a.intent.Record())
			}
		})
	}
}

func TestVerifyAnchorRequiresOwnedRecordAndFreshObservation(t *testing.T) {
	for _, name := range []string{"healthy", "legacy", "unpublished", "absent", "unavailable"} {
		t.Run(name, func(t *testing.T) {
			a := newAnchorFixture(t)
			a.prepare(t)
			r := a.intent.Record()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := port.MountRefused
			switch name {
			case "healthy":
				want = ""
			case "legacy":
				r.Schema = 1
			case "unpublished":
				r.Anchor.Stage = "intent"
			case "absent":
				delete(a.engine.Anchors, r.Anchor.Name)
			case "unavailable":
				cancel()
				want = port.MountUncertain
			}
			before := len(a.engine.Events)
			if got := verifyAnchor(ctx, r, a.connection); got != want {
				t.Fatalf("verify = %s, want %s", got, want)
			}
			for _, event := range a.engine.Events[before:] {
				if event != "anchor inspect" {
					t.Fatal("verification mutated resources", a.engine.Events)
				}
			}
		})
	}
}

func TestReleaseAnchorRetainsEvidenceWhenAuthorityChanges(t *testing.T) {
	for _, name := range []string{"healthy", "worker not absent", "intent adoption unsafe", "vanished before removal", "unsafe before removal", "unsafe after removal"} {
		t.Run(name, func(t *testing.T) {
			a := newAnchorFixture(t)
			a.prepare(t)
			r := a.intent.Record()
			if name == "intent adoption unsafe" {
				// Independently seed recovery at the lost-allocation-reply boundary.
				r.Anchor.Stage, r.Anchor.Created = "intent", ""
				a.intent.Close()
				ref := port.Reference(r.Project, r.NameToken())
				a.store.Records[ref] = a.store.Codec.Encode(r)
				var failure port.Outcome
				a.intent, failure = a.store.OpenCleanup(t.Context(), ref)
				if failure != "" {
					t.Fatal(failure)
				}
				t.Cleanup(a.intent.Close)
			}
			if name == "worker not absent" {
				r.Stage = "intent"
				if failure := a.intent.Save(r); failure != "" {
					t.Fatal(failure)
				}
			}
			before := a.intent.Record()
			inspections := 0
			a.engine.After = func(event string) {
				if event != "anchor inspect" {
					return
				}
				inspections++
				if name == "intent adoption unsafe" && inspections == 1 || name == "unsafe before removal" && inspections == 2 || name == "unsafe after removal" && inspections == 3 {
					a.store.UnsafeRecord = true
				}
				if name == "vanished before removal" && inspections == 2 {
					delete(a.engine.Anchors, r.Anchor.Name)
				}
			}
			want := port.Unsafe
			switch name {
			case "healthy":
				want = ""
			case "worker not absent":
				want = port.Refused
			case "vanished before removal":
				want = port.Incomplete
			}
			if got := releaseAnchor(t.Context(), a.intent, a.connection); got != want {
				t.Fatalf("release = %s, want %s", got, want)
			}
			if want != "" && a.intent.Record() != before {
				t.Fatal("failed release changed original evidence", a.intent.Record())
			}
			removed := slices.Contains(a.engine.Events, "anchor remove")
			if removed != (name == "healthy" || name == "unsafe after removal") {
				t.Fatal("removal crossed an authority failure", a.engine.Events)
			}
			if name == "healthy" && (len(a.engine.Anchors) != 0 || a.intent.Record().Anchor.Stage != "absent") {
				t.Fatal("successful release lacks independent absence", a.intent.Record())
			}
		})
	}
}

func TestAnchorAuthorityRejectsDifferentSessionAndUnsafeIntent(t *testing.T) {
	for _, name := range []string{"session", "intent"} {
		t.Run(name, func(t *testing.T) {
			a := newAnchorFixture(t)
			a.prepare(t)
			if name == "session" {
				a.sessionID = strings.Repeat("3", 64)
			}
			s, _ := a.openSession(t, false)
			ctx := t.Context()
			if name == "session" {
				// This session is valid on its own but cannot authorize another anchor.
				ctx = context.WithValue(ctx, recoveryAnchorKey{}, recoveryAnchorState{s, &port.RecoveryEntry{}})
				if got := anchorAuthority(ctx, s.Intent(), a.connection); got != port.Refused {
					t.Fatal("different session authorized anchor", got)
				}
				return
			}
			a.store.UnsafeRecord = true
			if got := anchorAuthority(ctx, s.Intent(), a.connection); got != port.Unsafe {
				t.Fatal("unsafe intent authorized anchor", got)
			}
		})
	}
}

func (a anchorFixture) openSession(t *testing.T, worker bool) (port.Session, *f.Registry) {
	t.Helper()
	if worker {
		r := a.intent.Record()
		r.Stage = "intent"
		if failure := a.intent.Save(r); failure != "" {
			t.Fatal(failure)
		}
		id, failure := a.connection.CreateRun(t.Context(), r, []string{"probe"})
		if failure != "" {
			t.Fatal(failure)
		}
		r.Stage, r.ContainerID = "owned", id
		if failure := a.intent.Save(r); failure != "" {
			t.Fatal(failure)
		}
		if failure := a.connection.Start(t.Context(), id); failure != "" {
			t.Fatal(failure)
		}
	}
	a.intent.Close()
	registry := &f.Registry{Store: a.store, Project: a.intent.Binding().Frozen.Project}
	lease, failure := registry.Register(t.Context(), approval.Durable{Frozen: a.intent.Binding().Frozen}, a.sessionID, a.intent.Record().NameToken())
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(lease.Close)
	s, failure := registry.Open(t.Context(), a.sessionID, false)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(s.Close)
	if worker {
		r := s.Record()
		r.State = "running"
		if failure := s.Save(r); failure != "" {
			t.Fatal(failure)
		}
	}
	return s, registry
}

func TestImmediateAnchorTeardownRetainsBothResourcesOnFailure(t *testing.T) {
	for _, name := range []string{"exit recorded", "exit publication", "stopping publication", "kill failed", "worker removal failed", "uncreated authority"} {
		t.Run(name, func(t *testing.T) {
			a := newAnchorFixture(t)
			a.prepare(t)
			s, registry := a.openSession(t, name != "uncreated authority")
			var exit *int
			want := port.IntentWrite
			switch name {
			case "exit recorded":
				exit = new(int(7))
				want = ""
			case "exit publication":
				exit = new(int(7))
				registry.SaveFailure = port.IntentWrite
			case "stopping publication":
				registry.SaveFailure = port.IntentWrite
			case "kill failed":
				a.engine.SignalFailure = port.EngineLost
				want = port.EngineLost
			case "worker removal failed":
				a.engine.Following = "still present"
				want = port.Incomplete
			case "uncreated authority":
				a.engine.After = func(event string) {
					if event == "anchor inspect" {
						a.store.UnsafeRecord = true
					}
				}
				want = port.Unsafe
			}
			if got := stopAnchoredSession(t.Context(), s, a.connection, "SIGTERM", exit, true, true); got != want {
				t.Fatalf("teardown = %s, want %s", got, want)
			}
			if name == "exit recorded" {
				if s.Record().WorkloadExit == nil || *s.Record().WorkloadExit != 7 || s.Record().State != "stopped" || len(a.engine.Resources) != 0 || len(a.engine.Anchors) != 0 {
					t.Fatal("immediate teardown lost exit or resources", s.Record())
				}
				return
			}
			if s.Record().State == "stopped" || s.Intent().Record().Anchor.Stage != "owned" || len(a.engine.Anchors) != 1 || slices.Contains(a.engine.Events, "anchor remove") {
				t.Fatal("failure discarded anchor retry evidence", s.Record(), a.engine.Events)
			}
			if name != "uncreated authority" && len(a.engine.Resources) != 1 {
				t.Fatal("failure removed the worker", a.engine.Events)
			}
		})
	}
}

func TestFinishSessionRequiresAnchorAbsence(t *testing.T) {
	a := newAnchorFixture(t)
	a.prepare(t)
	s, registry := a.openSession(t, true)
	writes := registry.Writes
	if got := finishSession(s); got != port.Refused || registry.Writes != writes || s.Intent().Record().Completed() {
		t.Fatal("worker-only completion discarded the owned anchor", got, s.Record())
	}
}

func TestDetectorRefusesMissingObservationPorts(t *testing.T) {
	a := newAnchorFixture(t)
	a.prepare(t)
	s, _ := a.openSession(t, true)
	for _, w := range []port.Workspace{struct{ port.Workspace }{a.workspace}, noProcessWorkspace{a.workspace}} {
		if detector, failure := detectAnchor(t.Context(), s.Intent(), a.connection, w); detector != port.DetectorRefused || failure != port.MountRefused {
			t.Fatal("unavailable observation ports verified a mount", detector, failure)
		}
	}
}

func TestLaunchRefusesChangedAnchorBeforeStart(t *testing.T) {
	for _, registered := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "registered"}[registered], func(t *testing.T) {
			runner, frozen, registry, engine, sessionID := registeredLifecycleFixture()
			if !registered {
				runner.Registry = nil
				runner.Entropy = f.Entropy{Value: strings.Repeat("1", 64)}
			}
			verifications := 0
			engine.After = func(event string) {
				if event != "verify" {
					return
				}
				verifications++
				if verifications == 2 {
					for name, o := range engine.Anchors {
						o.Source = "/proc/456/fd/9"
						engine.Anchors[name] = o
					}
				}
			}
			result := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{})
			if result.Outcome != port.MountRefused || result.Cleanup == port.Complete || result.Started || result.Exited || slices.Contains(engine.Events, "start") || slices.Contains(engine.Events, "anchor remove") || len(engine.Anchors) != 1 {
				t.Fatal("changed anchor authorized workload or foreign cleanup", result, engine.Events)
			}
			if registered && registry.Records[sessionID].State == "stopped" {
				t.Fatal("anchor conflict claimed completed recovery")
			}
		})
	}
}

func TestDirectRunRejectsUnavailableDescriptorBeforeCreate(t *testing.T) {
	runner, frozen, _, engine, _ := registeredLifecycleFixture()
	runner.Registry = nil
	runner.Entropy = f.Entropy{Value: strings.Repeat("1", 64)}
	runner.Workspace.(*f.Workspace).Descriptor = ""
	result := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{})
	if result.Outcome != port.MountRefused || result.Started || result.Exited || slices.Contains(engine.Events, "create") || len(engine.Anchors) != 0 {
		t.Fatal("unavailable descriptor authorized a worker", result, engine.Events)
	}
}
