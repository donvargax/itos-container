package main

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rfake "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sdk "github.com/donvargax/itos-container/internal/engine/session/docker"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

// These decorators record completed port calls, including their real failure and
// durability observations. They never supply a successful lifecycle result.
type offlineEvent struct {
	Operation, ID, Failure string
	Record                 sp.Record
	Binding                sp.Binding
	Argv                   []string
	Observation            sp.Observation
	Status                 sp.Inspection
	Exit                   int
	Writes                 []sessionContractEvent
}

type offlineObserver struct {
	t       *testing.T
	request sessionContractRequest
	ops     *sessionContractIO
	memory  *fake.RunEngine
	space   *fake.Workspace
	record  sp.Record
	binding sp.Binding
	events  []offlineEvent
	checks  int
	exited  bool
}

func (o *offlineObserver) event(e offlineEvent) {
	e.Record, e.Binding = o.record, o.binding
	e.Writes = slices.Clone(o.ops.events)
	if o.memory != nil {
		e.Writes = (&sessionMemoryOutput{Store: o.memory.Store}).events()
	}
	o.events = append(o.events, e)
}

func (o *offlineObserver) barrier(stage string) {
	writeSessionContract(o.t, o.request.Checkpoint+"-barrier", stage)
	var release string
	if err := json.NewDecoder(os.Stdin).Decode(&release); err != nil || release != "continue" {
		o.t.Fatal("offline barrier was not released")
	}
	o.event(offlineEvent{Operation: "fixture " + stage})
}

type offlineStore struct {
	sp.Store
	o *offlineObserver
}

func (s offlineStore) New(ctx context.Context, d ap.Durable, token string) (sp.Intent, sp.Outcome) {
	i, f := s.Store.New(ctx, d, token)
	return s.observed(i, f, "intent")
}

func (s offlineStore) OpenCleanup(ctx context.Context, ref string) (sp.Intent, sp.Outcome) {
	if s.o.exited && s.o.request.CleanupCause == "internal" {
		// An unclassified failure from a port, not a fabricated public rule/code.
		f := sp.Outcome("slice-13-private-adapter-failure")
		s.o.event(offlineEvent{Operation: "reopen", Failure: string(f)})
		return nil, f
	}
	i, f := s.Store.OpenCleanup(ctx, ref)
	return s.observed(i, f, "reopen")
}

func (s offlineStore) observed(i sp.Intent, f sp.Outcome, operation string) (sp.Intent, sp.Outcome) {
	if f == "" {
		s.o.record, s.o.binding = i.Record(), i.Binding()
	}
	s.o.event(offlineEvent{Operation: operation, Failure: string(f)})
	if f != "" {
		return i, f
	}
	return offlineIntent{Intent: i, o: s.o, launch: operation == "intent"}, f
}

type offlineIntent struct {
	sp.Intent
	o      *offlineObserver
	launch bool
}

func (i offlineIntent) Validate() sp.Outcome {
	o := i.o
	if i.launch {
		o.checks++
		stage := "before create"
		if o.checks == 2 {
			stage = "after create before start"
		}
		if o.checks <= 2 && o.request.LaunchBarrier == stage {
			o.barrier(stage)
			if o.memory != nil {
				if o.request.ReplacedPath == "workspace subdirectory" {
					o.space.Changed = true
				} else {
					o.memory.Store.Root = "/slice-13-replaced-checkout"
				}
			}
		}
	}
	f := i.Intent.Validate()
	o.event(offlineEvent{Operation: "validate", Failure: string(f)})
	return f
}

func (i offlineIntent) Save(r sp.Record) sp.Outcome {
	f := i.Intent.Save(r)
	i.o.record = i.Record()
	i.o.event(offlineEvent{Operation: "save", Failure: string(f)})
	return f
}

type offlineEngine struct {
	sp.RunEngine
	o *offlineObserver
}

func (e offlineEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	engine := e.RunEngine
	if e.o.exited && e.o.memory != nil && e.o.request.CleanupCause == "environment" {
		// Let the real session adapter translate the same underlying readiness
		// failure as the removed local socket; the readiness fake does no I/O.
		engine = sdk.Engine{Readiness: &rfake.Engine{Failure: rp.EngineUnavailable}, Selection: b.Endpoint}
	}
	c, f := engine.OpenRun(ctx, b)
	e.o.event(offlineEvent{Operation: "open", Failure: string(f)})
	if f != "" {
		return c, f
	}
	return offlineConnection{RunConnection: c, o: e.o}, f
}

type offlineConnection struct {
	sp.RunConnection
	o *offlineObserver
}

func (c offlineConnection) Check(ctx context.Context) sp.Outcome {
	f := c.RunConnection.Check(ctx)
	c.o.event(offlineEvent{Operation: "check", Failure: string(f)})
	return f
}

func (c offlineConnection) CreateRun(ctx context.Context, r sp.Record, argv []string) (string, sp.Outcome) {
	id, f := c.RunConnection.CreateRun(ctx, r, argv)
	c.o.event(offlineEvent{Operation: "create", ID: id, Failure: string(f), Argv: slices.Clone(argv)})
	return id, f
}

func (c offlineConnection) Inspect(ctx context.Context, id string) (sp.Observation, sp.Inspection) {
	o, status := c.RunConnection.Inspect(ctx, id)
	c.o.event(offlineEvent{Operation: "inspect", ID: id, Observation: o, Status: status})
	return o, status
}

func (c offlineConnection) VerifyRun(ctx context.Context, id string, argv []string) sp.Outcome {
	f := c.RunConnection.VerifyRun(ctx, id, argv)
	c.o.event(offlineEvent{Operation: "verify", ID: id, Failure: string(f), Argv: slices.Clone(argv)})
	return f
}

func (c offlineConnection) Attach(ctx context.Context, id string, streams sp.Streams) (sp.Attachment, sp.Outcome) {
	a, f := c.RunConnection.Attach(ctx, id, streams)
	c.o.event(offlineEvent{Operation: "attach", ID: id, Failure: string(f)})
	if f != "" {
		return a, f
	}
	return offlineAttachment{Attachment: a, o: c.o}, f
}

func (c offlineConnection) Start(ctx context.Context, id string) sp.Outcome {
	f := c.RunConnection.Start(ctx, id)
	c.o.event(offlineEvent{Operation: "start", ID: id, Failure: string(f)})
	return f
}

func (c offlineConnection) Wait(ctx context.Context, id string) (int, sp.Outcome) {
	exit, f := c.RunConnection.Wait(ctx, id)
	c.o.event(offlineEvent{Operation: "wait", ID: id, Failure: string(f), Exit: exit})
	if f == "" && c.o.request.CleanupCause != "" {
		c.o.barrier("after exit")
		c.o.exited = true
		if c.o.memory != nil {
			e := c.o.memory
			switch c.o.request.CleanupCause {
			case "ownership":
				e.Inspection = "different intent label"
			case "configuration":
				e.Store.Records[sp.Reference(c.o.record.Project, c.o.record.NameToken())] = []byte(`{"invalid":`)
			case "temporary":
				e.Following = "still present"
			}
		}
	}
	return exit, f
}

func (c offlineConnection) RemoveRun(ctx context.Context, id string) {
	c.RunConnection.RemoveRun(ctx, id)
	c.o.event(offlineEvent{Operation: "remove", ID: id})
}

type offlineAttachment struct {
	sp.Attachment
	o *offlineObserver
}

func (a offlineAttachment) Drain(ctx context.Context) sp.Outcome {
	f := a.Attachment.Drain(ctx)
	a.o.event(offlineEvent{Operation: "drain", Failure: string(f)})
	return f
}
