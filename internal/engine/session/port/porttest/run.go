package porttest

import (
	"context"
	"io"
	"slices"
	"sync"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// RunEngine is an in-memory lifecycle fake, including explicit boundary faults.
type RunEngine struct {
	eventMu sync.Mutex
	*Engine
	Boundary, AttachFailure, StartFailure, WaitFailure, DrainFailure port.Outcome
	OpenFailure                                                      port.Outcome
	Exit                                                             int
	Argv                                                             []string
	Events                                                           []string
	Output, ErrorOutput                                              []byte
	After                                                            func(string)
	AfterStart, AfterWait                                            func()
	WaitUntilCancel, IgnoreSignal                                    bool
	SignalFailure                                                    port.Outcome
	StdinClosed, DrainUntilCancel                                    bool
	DrainContextErr                                                  error
	WaitRelease                                                      <-chan struct{}
}
type runConnection struct {
	*memoryConnection
	run *RunEngine
}
type runAttachment struct {
	run      *RunEngine
	failure  port.Outcome
	failures chan port.Outcome
}

func (e *RunEngine) event(name string) {
	e.eventMu.Lock()
	e.Events = append(e.Events, name)
	e.eventMu.Unlock()
	if e.After != nil {
		e.After(name)
	}
}
func (e *RunEngine) OpenRun(ctx context.Context, b port.Binding) (port.RunConnection, port.Outcome) {
	if e.OpenFailure != "" {
		return nil, e.OpenFailure
	}
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
	a := runAttachment{run: c.run, failures: make(chan port.Outcome, 2)}
	if c.run.StdinClosed {
		a.failures <- port.StdinClosed
	} else if streams.In != nil {
		if _, err := io.Copy(io.Discard, streams.In); err != nil {
			a.failure = port.ExecutionFailed
		}
	}
	for _, output := range []struct {
		writer port.Writer
		data   []byte
	}{{streams.Out, c.run.Output}, {streams.Err, c.run.ErrorOutput}} {
		if output.writer != nil && len(output.data) != 0 {
			if n, err := output.writer.Write(output.data); err != nil || n != len(output.data) {
				a.failure = port.ExecutionFailed
			}
		}
	}
	if a.failure != "" {
		a.failures <- a.failure
	}
	return a, ""
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
	if c.run.AfterStart != nil {
		c.run.AfterStart()
	}
	return ""
}
func (c *runConnection) Wait(ctx context.Context, id string) (int, port.Outcome) {
	c.run.event("wait")
	if c.run.WaitRelease != nil {
		select {
		case <-c.run.WaitRelease:
		case <-ctx.Done():
		}
	}
	if c.run.WaitUntilCancel && c.engine.Resources[id].Running {
		<-ctx.Done()
	}
	if ctx.Err() != nil {
		return 0, port.ExecutionFailed
	}
	o := c.engine.Resources[id]
	o.Status = "exited"
	o.Running = false
	o.PID = 0
	c.engine.Resources[id] = o
	if c.run.AfterWait != nil {
		c.run.AfterWait()
	}
	return c.run.Exit, c.run.WaitFailure
}
func (c *runConnection) RemoveRun(ctx context.Context, id string) {
	c.run.event("remove")
	c.Remove(ctx, id)
}
func (c *runConnection) Signal(ctx context.Context, id, signal string) port.Outcome {
	c.run.event(signal)
	if ctx.Err() != nil {
		return port.ExecutionFailed
	}
	if c.run.SignalFailure != "" {
		return c.run.SignalFailure
	}
	if c.run.IgnoreSignal && signal != "SIGKILL" {
		return ""
	}
	o := c.engine.Resources[id]
	o.Running, o.Status, o.PID = false, "exited", 0
	c.engine.Resources[id] = o
	return ""
}
func (a runAttachment) Drain(ctx context.Context) port.Outcome {
	a.run.event("drain")
	if a.run.DrainUntilCancel {
		<-ctx.Done()
	}
	a.run.DrainContextErr = ctx.Err()
	if ctx.Err() != nil {
		return port.ExecutionFailed
	}
	if a.failure != "" {
		return a.failure
	}
	return a.run.DrainFailure
}
func (a runAttachment) Close()                        { a.run.event("detach") }
func (a runAttachment) Failures() <-chan port.Outcome { return a.failures }

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
