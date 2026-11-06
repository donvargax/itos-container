package porttest

import (
	"context"
	"slices"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// RunEngine is an in-memory lifecycle fake, including explicit boundary faults.
type RunEngine struct {
	*Engine
	Boundary, AttachFailure, StartFailure, WaitFailure, DrainFailure port.Outcome
	Exit                                                             int
	Argv                                                             []string
	Events                                                           []string
	Output, ErrorOutput                                              []byte
	After                                                            func(string)
}
type runConnection struct {
	*memoryConnection
	run *RunEngine
}
type runAttachment struct{ run *RunEngine }

func (e *RunEngine) event(name string) {
	e.Events = append(e.Events, name)
	if e.After != nil {
		e.After(name)
	}
}
func (e *RunEngine) OpenRun(ctx context.Context, b port.Binding) (port.RunConnection, port.Outcome) {
	c, f := e.Open(ctx, b)
	if f != "" {
		return nil, f
	}
	e.event("open")
	return &runConnection{c.(*memoryConnection), e}, ""
}
func (c *runConnection) CreateRun(ctx context.Context, r port.Record, argv []string) (string, port.Outcome) {
	c.run.Argv = slices.Clone(argv)
	c.run.event("create")
	id, f := c.Create(ctx, r)
	if f == "" {
		o := c.engine.Resources[id]
		o.Mounts = true
		c.engine.Resources[id] = o
	}
	return id, f
}
func (c *runConnection) VerifyRun(ctx context.Context, id string, argv []string) port.Outcome {
	c.run.event("verify")
	if ctx.Err() != nil {
		return port.IsolationInspection
	}
	if c.run.Boundary != "" {
		return c.run.Boundary
	}
	o, ok := c.engine.Resources[id]
	if !ok || o.Status != "created" || !slices.Equal(c.run.Argv, argv) {
		return port.IsolationUnverified
	}
	return ""
}
func (c *runConnection) Attach(_ context.Context, _ string, streams port.Streams) (port.Attachment, port.Outcome) {
	c.run.event("attach")
	if c.run.AttachFailure != "" {
		return nil, c.run.AttachFailure
	}
	if streams.Out != nil {
		_, _ = streams.Out.Write(c.run.Output)
	}
	if streams.Err != nil {
		_, _ = streams.Err.Write(c.run.ErrorOutput)
	}
	return runAttachment{c.run}, ""
}
func (c *runConnection) Start(ctx context.Context, id string) port.Outcome {
	c.run.event("start")
	if ctx.Err() != nil {
		return port.ExecutionFailed
	}
	if c.run.StartFailure != "" {
		return c.run.StartFailure
	}
	o := c.engine.Resources[id]
	o.Status = "running"
	o.Running = true
	o.Started = true
	o.PID = 123
	c.engine.Resources[id] = o
	return ""
}
func (c *runConnection) Wait(ctx context.Context, id string) (int, port.Outcome) {
	c.run.event("wait")
	if ctx.Err() != nil {
		return 0, port.ExecutionFailed
	}
	o := c.engine.Resources[id]
	o.Status = "exited"
	o.Running = false
	o.PID = 0
	c.engine.Resources[id] = o
	return c.run.Exit, c.run.WaitFailure
}
func (c *runConnection) RemoveRun(ctx context.Context, id string) {
	c.run.event("remove")
	c.Remove(ctx, id)
}
func (a runAttachment) Drain(context.Context) port.Outcome {
	a.run.event("drain")
	return a.run.DrainFailure
}
func (a runAttachment) Close() { a.run.event("detach") }

type Workspace struct {
	Failure port.Outcome
	Changed bool
	Closed  bool
}

func (w *Workspace) Pin(port.Binding) port.Outcome { w.Closed = false; return w.Failure }
func (w *Workspace) Unchanged() bool               { return !w.Changed && !w.Closed }
func (w *Workspace) Close()                        { w.Closed = true }

// RunEngineContract exercises the same frozen argv, independent observation and
// stream lifecycle against the memory implementation and the SDK HTTP fixture.
func RunEngineContract(t Reporter, open func() (port.RunEngine, port.Binding)) {
	t.Helper()
	e, b := open()
	ctx := context.Background()
	c, f := e.OpenRun(ctx, b)
	if f != "" {
		t.Errorf("open: %s", f)
		return
	}
	defer c.Close()
	argv := []string{"probe", "", "two words", "$HOME;*", "--", "--json"}
	id, f := c.CreateRun(ctx, b.Intent("1111111111111111111111111111111111111111111111111111111111111111"), argv)
	if f != "" {
		t.Errorf("create: %s", f)
		return
	}
	if o, status := c.Inspect(ctx, id); status != port.Observed || o.ID != id || !o.Mounts || o.Started {
		t.Errorf("bad unstarted observation: %#v %v", o, status)
	}
	if f = c.VerifyRun(ctx, id, argv); f != "" {
		t.Errorf("verify: %s", f)
		return
	}
	if f = c.VerifyRun(ctx, id, []string{"changed"}); f != port.IsolationUnverified {
		t.Errorf("changed argv accepted: %s", f)
	}
	a, f := c.Attach(ctx, id, port.Streams{})
	if f != "" {
		t.Errorf("attach: %s", f)
		return
	}
	defer a.Close()
	if f = c.Start(ctx, id); f != "" {
		t.Errorf("start: %s", f)
	}
	if exit, f := c.Wait(ctx, id); f != "" || exit != 7 {
		t.Errorf("wait: %d %s", exit, f)
	}
	if f = a.Drain(ctx); f != "" {
		t.Errorf("drain: %s", f)
	}
	c.RemoveRun(ctx, id)
	if _, status := c.Inspect(ctx, id); status != port.NotFound {
		t.Errorf("worker remains after removal")
	}
}
