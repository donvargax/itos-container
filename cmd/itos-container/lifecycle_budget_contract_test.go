package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	mp "github.com/donvargax/itos-container/internal/engine/manifest/port"
	mf "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	pp "github.com/donvargax/itos-container/internal/engine/policy/port"
	pf "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
	runport "github.com/donvargax/itos-container/internal/slices/run/port"
)

func TestLifecycleBudgetContract(t *testing.T) {
	name := os.Getenv("SLICE_12_BUDGET")
	if name == "" {
		t.Skip("selected by lifecycle scenarios")
	}
	for _, backend := range []string{"ports", "protected records"} {
		t.Run(backend, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				if name == "workspace dispatch" {
					lifecycleWorkspaceDispatchCase(t, os.Getenv("SLICE_12_ARGUMENT"), backend)
					return
				}
				if name == "workspace revalidation" {
					cause, phase, _ := strings.Cut(os.Getenv("SLICE_12_ARGUMENT"), "|")
					lifecycleWorkspaceRevalidationCase(t, cause, phase, backend)
					return
				}
				if name == "teardown wait" {
					lifecycleTeardownWaitCase(t, backend)
					return
				}
				if name == "startup lifetime" {
					cause, operation, _ := strings.Cut(os.Getenv("SLICE_12_ARGUMENT"), "|")
					lifecycleStartupLifetimeCase(t, cause, operation, backend)
					return
				}
				if name == "checkout lifetime" {
					cause, operation, _ := strings.Cut(os.Getenv("SLICE_12_ARGUMENT"), "|")
					lifecycleCheckoutLifetimeCase(t, cause, operation, backend)
					return
				}
				lifecycleBudgetCase(t, name, os.Getenv("SLICE_12_ARGUMENT"), backend)
			})
		})
	}
}

type lifecyclePreflightFault struct {
	stall                       string
	progress                    time.Duration
	events                      []string
	block                       func(string)
	checkout                    *rf.Checkout
	pending, closedWhilePending bool
	closes                      int
}

func (f *lifecyclePreflightFault) step(name string) {
	f.events = append(f.events, name)
	if f.block != nil {
		f.block(name)
	}
	if f.stall == name {
		time.Sleep(11 * time.Second)
		return
	}
	if f.progress > 0 {
		d := min(f.progress, 9*time.Second)
		f.progress -= d
		time.Sleep(d)
	}
}
func lifecycleBudgetPreflight(l *lifecycleLocal, f *lifecyclePreflightFault) (runport.Approval, ap.Snapshots) {
	s := rf.HealthySession()
	image := s.Images[rf.Reference]
	image.Label = mp.Label{}
	s.Images[rf.Reference] = image
	e := &rf.Engine{Session: s}
	checkout := &rf.Checkout{Identity: *af.Ready().Checkout}
	f.checkout = checkout
	i := readiness.Inspector{
		Checkout: lifecycleCheckout{checkout, f}, Engine: lifecycleReadinessEngine{e, f},
		Policy:     lifecyclePolicy{&pf.Source{Value: pp.Value{Kind: pp.Object, Fields: map[string]pp.Value{"version": {Kind: pp.Number, Number: pp.Decimal{Coefficient: "1", Exponent: "0"}}, "manifest": {Kind: pp.String, Text: "worker.json"}}}}, f},
		Files:      lifecycleManifest{&mf.Files{Data: map[string][]byte{"worker.json": []byte("manifest data")}}, f},
		Parser:     &mf.Parser{Value: mp.Value{Kind: mp.Object, Fields: map[string]mp.Value{"image": {Kind: mp.String, Text: rf.Reference}}}},
		References: &mf.References{Images: map[string]bool{rf.Reference: true}}, Digests: rf.Digests{rf.Reference: rf.Reference},
	}
	local := inspection.Readiness{Inspector: i, Input: rp.Input{OS: "linux", Architecture: "amd64", Root: "/checkout", Host: s.Address}}
	codec := &af.Codec{Values: map[string]ap.Candidate{"1000:1000": l.m.c}, Local: l.m.c}
	store := &af.Store{Records: map[string]ap.Record{l.m.c.Project: {Candidate: l.m.c, Generation: "1"}}}
	service := approval.Service{Readiness: local, Host: lifecycleHost{&af.Host{UID: "1000", GID: "1000", Stamp: "socket"}, f}, Codec: codec, Store: lifecycleApprovalStore{store, f}}
	return approval.Preflight{Service: service, Local: local, Canonical: codec}, lifecycleSnapshots{&af.Snapshots{}, f}
}

type lifecycleCheckout struct {
	rp.Checkout
	f *lifecyclePreflightFault
}

func (c lifecycleCheckout) Pin(root string) rp.Failure {
	c.f.step("checkout pin")
	return c.Checkout.Pin(root)
}
func (c lifecycleCheckout) Inspect() (rp.CheckoutIdentity, rp.Failure) {
	c.f.step("checkout inspection")
	return c.Checkout.Inspect()
}
func (c lifecycleCheckout) Unchanged() bool {
	c.f.step("checkout revalidation")
	return c.Checkout.Unchanged()
}
func (c lifecycleCheckout) Close() {
	c.f.closes++
	c.f.closedWhilePending = c.f.closedWhilePending || c.f.pending
	c.Checkout.Close()
}

type lifecyclePolicy struct {
	pp.Source
	f *lifecyclePreflightFault
}

func (s lifecyclePolicy) Read() (pp.Value, []pp.Issue) {
	s.f.step("policy read")
	return s.Source.Read()
}

type lifecycleManifest struct {
	mp.Files
	f *lifecyclePreflightFault
}

func (s lifecycleManifest) Read(path string) ([]byte, mp.Failure) {
	s.f.step("manifest read")
	return s.Files.Read(path)
}

type lifecycleReadinessEngine struct {
	rp.Engine
	f *lifecyclePreflightFault
}

func (e lifecycleReadinessEngine) Open(selection, root string, d time.Duration) (rp.Session, rp.Failure) {
	e.f.step("engine connection")
	s, failure := e.Engine.Open(selection, root, d)
	if s == nil {
		return nil, failure
	}
	return lifecycleReadinessSession{s, e.f}, failure
}

type lifecycleReadinessSession struct {
	rp.Session
	f *lifecyclePreflightFault
}

func (s lifecycleReadinessSession) Version() (rp.Version, rp.Failure) {
	s.f.step("engine version")
	return s.Session.Version()
}
func (s lifecycleReadinessSession) Info() (rp.Info, rp.Failure) {
	s.f.step("preflight engine inspection")
	return s.Session.Info()
}
func (s lifecycleReadinessSession) Image(ref string) (rp.Image, rp.Failure) {
	s.f.step("image inspection")
	return s.Session.Image(ref)
}
func (s lifecycleReadinessSession) Unchanged() bool {
	s.f.step("socket revalidation")
	return s.Session.Unchanged()
}

type lifecycleHost struct {
	ap.Host
	f *lifecyclePreflightFault
}

func (h lifecycleHost) Identity() (string, string) {
	h.f.step("host identity")
	return h.Host.Identity()
}
func (h lifecycleHost) Socket(s string) string { h.f.step("socket identity"); return h.Host.Socket(s) }

type lifecycleApprovalStore struct {
	ap.Store
	f *lifecyclePreflightFault
}

func (s lifecycleApprovalStore) Read(c ap.Candidate) (ap.Record, ap.Failure) {
	s.f.step("preflight approval state read")
	return s.Store.Read(c)
}
func (s lifecycleApprovalStore) Select(c ap.Candidate, g string) (ap.Frozen, ap.Failure) {
	s.f.step("approval selection")
	return s.Store.Select(c, g)
}

type lifecycleSnapshots struct {
	ap.Snapshots
	f *lifecyclePreflightFault
}

func (s lifecycleSnapshots) Publish(f ap.Frozen) (ap.Durable, ap.Failure) {
	s.f.step("frozen snapshot publication")
	return s.Snapshots.Publish(f)
}

type lifecycleIntentStore struct {
	sp.Store
	l     *lifecycleLocal
	stage string
}

func (s lifecycleIntentStore) Prepare(ctx context.Context, f ap.Durable, token string) (sp.Intent, sp.Outcome) {
	if s.stage == "intent lock acquisition" {
		s.l.event(ctx, s.stage, "")
	}
	i, failure := s.Store.Prepare(ctx, f, token)
	if s.stage == "intent publication" {
		s.l.event(ctx, s.stage, "")
	}
	return i, failure
}

func lifecycleBudgetCase(t *testing.T, name, operation, backend string) {
	l := lifecycleLocalFixture(t, backend)
	f := &lifecyclePreflightFault{}
	if name == "startup" {
		f.progress = 115 * time.Second
	} else {
		f.stall = operation
	}
	l.approval, l.snapshots = lifecycleBudgetPreflight(l, f)
	stage := map[string]string{"create response": "create-reply", "boundary inspection": "verify", "attach establishment": "attach", "start acknowledgement": "start-reply"}[operation]
	if strings.HasPrefix(operation, "intent ") {
		stage = operation
		l.store = lifecycleIntentStore{l.store, l, operation}
	}
	if name == "startup" {
		stage = "attach"
	}
	fired, stoppedAt := false, time.Time{}
	var callBudget time.Duration
	l.hook = func(ctx context.Context, event string) {
		if event != stage || fired {
			return
		}
		fired = true
		d, _ := ctx.Deadline()
		callBudget = time.Until(d)
		<-ctx.Done()
		stoppedAt = time.Now()
		if event == "create-reply" || event == "start-reply" {
			l.hidden, l.unknownExit = true, true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	code, diagnostic, _ := l.invoke(ctx)
	elapsed := time.Since(l.start)
	wantDeadline, rule := 10*time.Second, "run.call-timeout"
	if name == "startup" {
		wantDeadline, rule = 120*time.Second, "run.startup-timeout"
	}
	if code != 75 || !strings.Contains(diagnostic, rule) || (name != "startup" && !strings.Contains(diagnostic, operation)) {
		t.Errorf("finite budget result: exit %d at %s, %s", code, elapsed, diagnostic)
	}
	if elapsed < wantDeadline || elapsed > wantDeadline+30*time.Second {
		t.Error("invocation lost phase/cleanup deadline", elapsed)
	}
	if stage != "" && (!fired || stoppedAt.Sub(l.start) != wantDeadline || callBudget > 10*time.Second) {
		t.Error("operation wait/deadline wrong", fired, stoppedAt.Sub(l.start), callBudget)
	}
	if name == "startup" && (f.progress != 0 || callBudget != 5*time.Second) {
		t.Error("startup did not include successful preflight progress", f.progress, callBudget)
	}
	if l.count("start") != 0 && stage != "start-reply" {
		t.Error("late reply launched work", l.events)
	}
	l.hook = nil
	if l.count("create") > 0 {
		r := (domain.Recovery{Registry: l.registry, Engine: lifecycleEngine{l}}).Status(context.Background())
		if len(r.Entries) != 1 || r.Entries[0].Project != l.m.c.Project {
			t.Error("accepted mutation lost original protected identity", r)
		}
	}
	beforeLate := l.count("start")
	time.Sleep(11 * time.Second)
	synctest.Wait()
	if l.count("start") != beforeLate {
		t.Error("late reply resumed launch")
	}
	// The same complete assembly must succeed when only the injected fault clears.
	control := lifecycleLocalFixture(t, backend)
	control.approval, control.snapshots = lifecycleBudgetPreflight(control, &lifecyclePreflightFault{})
	code, diagnostic, _ = control.invoke(context.Background())
	if code != 0 || diagnostic != "" || control.count("create") != 1 || control.count("start") != 1 || control.count("remove") != 1 || len(control.m.e.Resources) != 1 {
		t.Error("healthy control failed", code, diagnostic, control.events)
	}
}
