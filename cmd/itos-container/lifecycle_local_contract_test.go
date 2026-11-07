package main

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runport "github.com/donvargax/itos-container/internal/slices/run/port"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

// The feature subprocess selects one complete case. Pending scenarios do not
// run in ordinary unit gates until their assertions have supplied red evidence.
func TestLifecycleLocalContract(t *testing.T) {
	name := os.Getenv("SLICE_12_LOCAL")
	if name == "" {
		t.Skip("selected by lifecycle scenarios")
	}
	for _, backend := range []string{"ports", "protected records"} {
		t.Run(backend, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				lifecycleLocalCase(t, name, os.Getenv("SLICE_12_ARGUMENT"), os.Getenv("SLICE_12_SIGNAL"), backend)
			})
		})
	}
}

type lifecycleEvent struct {
	name, target string
	at           time.Time
	remaining    time.Duration
}
type lifecycleLocal struct {
	approval            runport.Approval
	snapshots           ap.Snapshots
	m                   publicMemory
	store               sp.Store
	registry            sp.Registry
	engine              sp.RunEngine
	workspace           sp.Workspace
	eventMu             sync.Mutex
	events              []lifecycleEvent
	hook                func(context.Context, string)
	hidden, unknownExit bool
	start               time.Time
}

func lifecycleLocalFixture(t *testing.T, backend string) *lifecycleLocal {
	m := publicMemoryFixture(t)
	l := &lifecycleLocal{m: m, store: m.registry.Store, registry: m.registry, start: time.Now()}
	if backend == "ports" {
		memory := &sf.Store{Codec: m.registry.Store.Codec, UID: "1000", GID: "1000", Root: m.c.Root,
			Snapshots: map[string][]byte{"snapshots/" + m.c.Project + "/1.json": m.storage.Directories["snapshots/"+m.c.Project].Files["1.json"]}}
		l.store, l.registry = memory, &sf.Registry{Store: memory, Project: m.c.Project}
	}
	l.m.e.WaitFailure = ""
	// An independently seeded resource must survive every run and recovery.
	l.m.e.Resources[strings.Repeat("f", 64)] = sp.Observation{ID: strings.Repeat("f", 64), Name: "unrelated", ImageID: "unrelated"}
	return l
}
func (l *lifecycleLocal) event(ctx context.Context, name, target string) {
	d, _ := ctx.Deadline()
	l.eventMu.Lock()
	l.events = append(l.events, lifecycleEvent{name, target, time.Now(), time.Until(d)})
	l.eventMu.Unlock()
	if l.hook != nil {
		l.hook(ctx, name)
	}
}
func (l *lifecycleLocal) count(name string) int {
	l.eventMu.Lock()
	defer l.eventMu.Unlock()
	n := 0
	for _, e := range l.events {
		if e.name == name {
			n++
		}
	}
	return n
}
func (l *lifecycleLocal) invoke(ctx context.Context) (int, string, string) {
	var out bytes.Buffer
	errout := &lifecycleDiagnostic{}
	code := l.invokeWithStreams(ctx, &out, errout)
	diagnostic := errout.String()
	// Dispose of abandoned operations only after recording return-time output.
	synctest.Wait()
	return code, diagnostic, out.String()
}

func (l *lifecycleLocal) invokeWithStreams(ctx context.Context, out, errout sp.Writer) int {
	approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: l.m.c, Record: ap.Record{Candidate: l.m.c, Generation: "1"}, Status: "matching"}}
	engine := l.engine
	if engine == nil {
		engine = lifecycleEngine{l}
	}
	workspace := l.workspace
	if workspace == nil {
		workspace = &sf.Workspace{}
	}
	runner := domain.Runner{Store: l.store, Registry: lifecycleRegistry{l.registry, l}, Engine: engine, Workspace: workspace, Entropy: &publicMemoryEntropy{}}
	var selected runport.Approval = lifecycleApproval{approval, l, ctx}
	if l.approval != nil {
		selected = l.approval
	}
	snapshots := l.snapshots
	if snapshots == nil {
		snapshots = &af.Snapshots{}
	}
	return (runcommand.CLI{Context: ctx, Approval: selected, Snapshots: snapshots, Runner: runner, Args: []string{"--", "synthetic-sensitive-argv"}}).Run(&cli.UI{Stdout: out, Stderr: errout, Classify: runcommand.Classify})
}

type lifecycleApproval struct {
	*runfake.Approval
	l   *lifecycleLocal
	ctx context.Context
}

func (a lifecycleApproval) Inspect() ap.View {
	a.l.event(a.ctx, "preflight", "")
	return a.Approval.Inspect()
}

type lifecycleRegistry struct {
	sp.Registry
	l *lifecycleLocal
}

func (r lifecycleRegistry) ForRun(d ap.Durable) sp.Registry {
	if bound, ok := r.Registry.(sp.RunRegistry); ok {
		r.Registry = bound.ForRun(d)
	}
	return r
}
func (r lifecycleRegistry) Open(ctx context.Context, id string, ro bool) (sp.Session, sp.Outcome) {
	r.l.event(ctx, "session-open", id)
	s, f := r.Registry.Open(ctx, id, ro)
	if s == nil {
		return nil, f
	}
	return lifecycleSession{s, r.l, ctx}, f
}

type lifecycleSession struct {
	sp.Session
	l   *lifecycleLocal
	ctx context.Context
}

func (s lifecycleSession) Save(r sp.PublicRecord) sp.Outcome {
	f := s.Session.Save(r)
	if f == "" && r.State == "ready" {
		s.l.event(s.ctx, "owned", r.ID)
	}
	return f
}

type lifecycleEngine struct{ l *lifecycleLocal }

func (e lifecycleEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	e.l.event(ctx, "open", b.EngineID)
	c, f := e.l.m.e.OpenRun(ctx, b)
	if c == nil {
		return nil, f
	}
	return lifecycleConnection{c, e.l}, f
}

type lifecycleConnection struct {
	sp.RunConnection
	l *lifecycleLocal
}

func (c lifecycleConnection) Check(ctx context.Context) sp.Outcome {
	c.l.event(ctx, "check", "")
	return c.RunConnection.Check(ctx)
}
func (c lifecycleConnection) Inspect(ctx context.Context, id string) (sp.Observation, sp.Inspection) {
	c.l.event(ctx, "inspect", id)
	if c.l.hidden {
		if c.l.unknownExit {
			return sp.Observation{}, sp.Unavailable
		}
		return sp.Observation{}, sp.NotFound
	}
	return c.RunConnection.Inspect(ctx, id)
}
func (c lifecycleConnection) CreateRun(ctx context.Context, r sp.Record, argv []string) (string, sp.Outcome) {
	c.l.event(ctx, "create", r.Name)
	id, f := c.RunConnection.CreateRun(ctx, r, argv)
	c.l.event(ctx, "create-reply", id)
	if ctx.Err() != nil {
		return "", sp.Uncertain
	}
	return id, f
}
func (c lifecycleConnection) VerifyRun(ctx context.Context, id string, argv []string) sp.Outcome {
	c.l.event(ctx, "verify", id)
	f := c.RunConnection.VerifyRun(ctx, id, argv)
	c.l.event(ctx, "verified", id)
	return f
}
func (c lifecycleConnection) Attach(ctx context.Context, id string, streams sp.Streams) (sp.Attachment, sp.Outcome) {
	c.l.event(ctx, "attach", id)
	a, f := c.RunConnection.Attach(ctx, id, streams)
	c.l.event(ctx, "attached", id)
	return a, f
}
func (c lifecycleConnection) Start(ctx context.Context, id string) sp.Outcome {
	c.l.event(ctx, "start", id)
	f := c.RunConnection.Start(ctx, id)
	c.l.event(ctx, "start-reply", id)
	if ctx.Err() != nil {
		return sp.ExecutionFailed
	}
	return f
}
func (c lifecycleConnection) Wait(ctx context.Context, id string) (int, sp.Outcome) {
	c.l.event(ctx, "wait", id)
	if c.l.unknownExit {
		return 0, sp.ExecutionFailed
	}
	return c.RunConnection.Wait(ctx, id)
}
func (c lifecycleConnection) Signal(ctx context.Context, id, signal string) sp.Outcome {
	c.l.event(ctx, signal, id)
	return c.RunConnection.Signal(ctx, id, signal)
}
func (c lifecycleConnection) Remove(ctx context.Context, id string) {
	c.l.event(ctx, "remove", id)
	c.RunConnection.Remove(ctx, id)
}

func lifecycleLocalCase(t *testing.T, name, arg, signal, backend string) {
	l := lifecycleLocalFixture(t, backend)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	// A test-only outer guard prevents a broken lifecycle from hanging synctest.
	ctx, guard := context.WithTimeout(ctx, 180*time.Second)
	defer guard()
	want, rule, outcome := 75, "run.call-timeout", ""
	barrier, fired := "", false
	barrierIndex := 0
	var interrupted time.Time
	if signal == "" {
		signal = "SIGTERM"
	}
	switch name {
	case "start-timeout":
		barrier = "start-reply"
		l.m.e.Exit = 17
		if arg == "authoritative primary exit 17" {
			want = 17
		} else {
			l.unknownExit = true
		}
	case "cancel":
		rule, outcome, want = "run.cancelled", "not-started", 143
		if signal == "SIGINT" {
			want = 130
		}
		barrier = map[string]string{"before preflight": "preflight", "before create": "open", "after ownership publication": "owned", "after boundary verification": "verified", "after attach before start": "attached"}[arg]
	case "accepted":
		rule, want, outcome = "run.cancelled", 143, "not-started"
		barrier = arg + "-reply"
		if arg == "start" {
			outcome, l.unknownExit = "unknown", true
		}
	case "lost-signal":
		rule, want, outcome = "run.cancelled", 143, "unknown"
		if signal == "SIGINT" {
			want = 130
		}
		barrier, l.unknownExit = "start-reply", true
		l.m.e.IgnoreSignal = true
	case "teardown":
		barrier, rule = "wait", "run.teardown-timeout"
		child, err := strconv.Atoi(arg)
		if err != nil {
			t.Fatal(err)
		}
		l.m.e.Exit, l.m.e.Output = child, []byte("available trailing bytes")
		if child != 0 {
			want = child
		}
	default:
		t.Fatal("unknown lifecycle case", name)
	}
	l.hook = func(call context.Context, event string) {
		if event == barrier && !fired {
			fired = true
			barrierIndex = len(l.events)
			switch name {
			case "start-timeout":
				<-call.Done()
				if l.unknownExit {
					l.hidden = true
				}
			case "cancel", "accepted", "lost-signal":
				interrupted = time.Now()
				cancel(sp.Interrupt{Signal: signal})
				if name == "accepted" {
					l.hidden = true
				}
			}
		}
		if name == "teardown" && fired && (event == "check" || event == "inspect" || event == "remove") {
			select {
			case <-time.After(9 * time.Second):
			case <-call.Done():
			}
		}
		if name == "lost-signal" {
			if event == signal {
				select {
				case <-time.After(10 * time.Second):
				case <-call.Done():
				}
			}
			if event == "SIGKILL" {
				l.hidden = true
			}
		}
	}
	if name == "teardown" {
		l.m.e.After = func(event string) {
			if event == "drain" {
				time.Sleep(9 * time.Second)
			}
		}
	}
	code, diagnostic, out := l.invoke(ctx)
	elapsed := time.Since(l.start)
	if !fired {
		t.Error("product did not reach lifecycle barrier", barrier)
	}
	if code != want || !strings.Contains(diagnostic, rule) || (outcome != "" && !strings.Contains(diagnostic, "workload outcome "+outcome)) {
		t.Errorf("lifecycle result: exit %d, want %d; elapsed %s; %s", code, want, elapsed, diagnostic)
	}
	if strings.Contains(diagnostic, "synthetic-sensitive-argv") {
		t.Error("sensitive argv leaked")
	}
	if name == "cancel" {
		if l.count("start") != 0 || l.count("SIGTERM") != 0 || l.count("SIGINT") != 0 || l.count("SIGKILL") != 0 {
			t.Error("cancelled startup entered a later phase or signalled worker", l.events)
		}
		for _, e := range l.events[barrierIndex:] {
			if !e.at.Before(interrupted) && slices.Contains([]string{"create", "verify", "attach"}, e.name) {
				t.Error("later startup phase after cancellation", e.name)
			}
		}
	}
	if name == "start-timeout" && (elapsed > 40*time.Second || l.count("create") != 1 || l.count("start") != 1) {
		t.Error("start timeout reset or duplicated launch", elapsed, l.events)
	}
	if name == "teardown" {
		// Writable diagnostics complete within the second reserved from cleanup.
		if elapsed != 29*time.Second || out != "available trailing bytes" {
			t.Error("teardown lost deadline or trailing bytes", elapsed, out)
		}
		found := false
		for _, e := range l.events[barrierIndex:] {
			if e.name == "session-open" && e.at.Sub(l.start) < 9*time.Second {
				found = true
			}
		}
		if !found {
			t.Error("drain blocked owned reconciliation")
		}
	}
	if name == "lost-signal" {
		if l.count(signal) != 1 || l.count("SIGKILL") != 1 {
			t.Error("signal request repeated or escalation missing", l.events)
		}
		for _, e := range l.events {
			if e.name == "SIGKILL" && e.at.Sub(interrupted) != 10*time.Second {
				t.Error("grace restarted after acknowledgement", e.at.Sub(interrupted))
			}
		}
	}
	if elapsed > 40*time.Second && name != "start-timeout" {
		t.Error("controller exceeded teardown budget", elapsed)
	}
	// Clear only observation faults, then use a new recovery controller. The
	// original protected IDs, snapshot, engine resource and bindings are retained.
	l.hook = nil
	id := strings.Repeat("a", 64)
	if name == "accepted" || name == "lost-signal" || name == "teardown" || (name == "start-timeout" && l.unknownExit) {
		e, f := l.registry.Open(context.Background(), id, true)
		if f != "" {
			t.Fatal("lost protected recovery", f)
		}
		r := e.Record()
		if r.State == "stopped" || (name != "teardown" && r.WorkloadExit != nil) {
			t.Error("uncertainty fabricated completion", r)
		}
		if name == "teardown" && (r.WorkloadExit == nil || strconv.Itoa(*r.WorkloadExit) != arg) {
			t.Error("observed exit lost", r)
		}
		if e.Intent().Record().Stage == "absent" {
			t.Error("unfenced absence published")
		}
		e.Close()
	}
	l.hidden, l.unknownExit = false, false
	l.m.e.IgnoreSignal = false
	beforeCreate, beforeStart := l.count("create"), l.count("start")
	if len(l.m.e.Resources) > 1 {
		r := (domain.Recovery{Registry: l.registry, Engine: lifecycleEngine{l}}).Stop(context.Background(), id)
		if len(r.Failures) != 0 || len(r.Entries) != 1 || r.Entries[0].Cleanup != "complete" {
			t.Error("fresh recovery lost original resource", r)
		}
	}
	if l.count("create") != beforeCreate || l.count("start") != beforeStart || len(l.m.e.Resources) != 1 {
		t.Error("recovery restarted or lost worker", l.events)
	}
	if !reflect.DeepEqual(l.m.e.Resources[strings.Repeat("f", 64)], sp.Observation{ID: strings.Repeat("f", 64), Name: "unrelated", ImageID: "unrelated"}) {
		t.Error("unrelated resource changed")
	}
}
