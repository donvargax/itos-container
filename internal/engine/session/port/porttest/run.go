package porttest

import (
	"context"
	"io"
	"slices"
	"sync"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
)

// RunEngine is an in-memory lifecycle fake, including explicit boundary faults.
type RunEngine struct {
	CredentialStaging                   *CredentialStager
	CredentialMounts                    []port.CredentialMount
	CredentialPaths, CredentialAcquired []byte
	OpenNil                             bool
	AfterProxyWait                      func()
	Networks                            map[string]port.NetworkObservation
	EgressFault                         string
	ProxyOutput, ProxyErrorOutput       string
	ProxyExit                           <-chan struct{}
	ProxyID                             string
	Anchors                             map[string]port.AnchorObservation
	eventMu                             sync.Mutex
	*Engine
	Boundary, AttachFailure, StartFailure, WaitFailure, DrainFailure port.Outcome
	OpenFailure                                                      port.Outcome
	Exit                                                             int
	Argv                                                             []string
	Events                                                           []string
	Output, ErrorOutput                                              []byte
	After                                                            func(string)
	AfterStart, AfterWait                                            func()
	AfterCancelledWait                                               func()
	BeforeResize                                                     func()
	ResizeFailure                                                    port.Outcome
	WaitUntilCancel, IgnoreSignal                                    bool
	SignalFailure                                                    port.Outcome
	StdinClosed, DrainUntilCancel                                    bool
	// StdinFailures lets tests order input closure after a lifecycle barrier.
	// Nil retains the immediate StdinClosed behavior used by stream contracts.
	StdinFailures   <-chan port.Outcome
	DrainContextErr error
	WaitRelease     <-chan struct{}
}
type runConnection struct {
	*memoryConnection
	run      *RunEngine
	terminal terminal.Dimensions
}
type runAttachment struct {
	run        *RunEngine
	failure    port.Outcome
	failures   chan port.Outcome
	outputDone chan struct{}
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
	if e.OpenNil {
		return nil, ""
	}
	if e.OpenFailure != "" {
		return nil, e.OpenFailure
	}
	c, f := e.Open(ctx, b)
	if f != "" {
		return nil, f
	}
	e.event("open")
	return &runConnection{memoryConnection: c.(*memoryConnection), run: e}, ""
}
func (c *runConnection) CreateRun(ctx context.Context, r port.Record, argv []string) (string, port.Outcome) {
	if !r.ValidCredentials(c.binding) || r.Schema == 4 && !r.Credentials.Ready() {
		return "", port.CredentialRefused
	}
	c.run.CredentialMounts = r.Credentials.Mounts()
	c.run.Argv = slices.Clone(argv)
	c.run.event("create")
	id, f := c.Create(ctx, r)
	if f == "" {
		c.engine.resourcesMu.Lock()
		o := c.engine.Resources[id]
		o.Mounts = true
		o.AnchorName, o.AnchorExact = r.Anchor.Name, r.Schema >= 2
		for _, m := range r.Credentials.Mounts() {
			o.CredentialMounts[port.CredentialIndex(m.Kind)] = m
		}
		o.CredentialExact = true
		c.engine.Resources[id] = o
		c.engine.resourcesMu.Unlock()
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
	c.engine.resourcesMu.Lock()
	o, ok := c.engine.Resources[id]
	c.engine.resourcesMu.Unlock()
	expected := [2]port.CredentialMount{}
	for _, m := range c.run.CredentialMounts {
		expected[port.CredentialIndex(m.Kind)] = m
	}
	if !ok || o.Status != "created" || !slices.Equal(c.run.Argv, argv) || o.CredentialMounts != expected || len(c.run.CredentialMounts) > 0 && (!o.CredentialExact || !o.AnchorExact || o.ImageID != c.binding.ImageID) {
		return port.IsolationUnverified
	}
	return ""
}
func (c *runConnection) Attach(_ context.Context, _ string, streams port.Streams) (port.Attachment, port.Outcome) {
	c.run.event("attach")
	if c.run.AttachFailure != "" {
		return nil, c.run.AttachFailure
	}
	a := &runAttachment{run: c.run, failures: make(chan port.Outcome, 2), outputDone: make(chan struct{})}
	if c.run.StdinClosed {
		a.failures <- port.StdinClosed
	} else if streams.In != nil {
		if _, err := io.Copy(io.Discard, streams.In); err != nil {
			a.failure = port.ExecutionFailed
		}
	}
	if c.terminal.Valid() {
		streams.Err = streams.Out
	}
	go a.forwardOutput(streams)
	return a, ""
}

func (a *runAttachment) forwardOutput(streams port.Streams) {
	defer close(a.outputDone)
	for _, output := range []struct {
		writer port.Writer
		data   []byte
	}{{streams.Out, a.run.Output}, {streams.Err, a.run.ErrorOutput}} {
		if output.writer != nil && len(output.data) != 0 {
			if n, err := output.writer.Write(output.data); err != nil || n != len(output.data) {
				a.failure = port.ExecutionFailed
			}
		}
	}
	if a.failure != "" {
		a.failures <- a.failure
	}
}
func (c *runConnection) Start(ctx context.Context, id string) port.Outcome {
	if c.isProxy(id) {
		return c.startProxy(ctx, id)
	}
	c.run.event("start")
	if ctx.Err() != nil {
		return port.ExecutionFailed
	}
	if len(c.run.CredentialMounts) > 0 {
		c.run.CredentialAcquired = slices.Clone(c.run.CredentialPaths)
		if c.run.CredentialStaging != nil {
			c.run.CredentialAcquired = nil
			for _, m := range c.run.CredentialMounts {
				c.run.CredentialAcquired = append(c.run.CredentialAcquired, c.run.CredentialStaging.Files[m.Source]...)
			}
		}
	}
	if c.run.StartFailure != "" {
		return c.run.StartFailure
	}
	c.engine.resourcesMu.Lock()
	o := c.engine.Resources[id]
	o.Status = "running"
	o.Running = true
	o.Started = true
	o.PID = 123
	c.engine.Resources[id] = o
	c.engine.resourcesMu.Unlock()
	if c.run.AfterStart != nil {
		c.run.AfterStart()
	}
	return ""
}
func (c *runConnection) Wait(ctx context.Context, id string) (int, port.Outcome) {
	if c.isProxy(id) {
		return c.waitProxy(ctx, id)
	}
	c.run.event("wait")
	if c.run.WaitRelease != nil {
		select {
		case <-c.run.WaitRelease:
		case <-ctx.Done():
		}
	}
	c.engine.resourcesMu.Lock()
	running := c.engine.Resources[id].Running
	c.engine.resourcesMu.Unlock()
	if c.run.WaitUntilCancel && running {
		<-ctx.Done()
	}
	if ctx.Err() != nil {
		if c.run.AfterCancelledWait != nil {
			c.run.AfterCancelledWait()
		}
		return 0, port.ExecutionFailed
	}
	c.engine.resourcesMu.Lock()
	o := c.engine.Resources[id]
	o.Status = "exited"
	o.Running = false
	o.PID = 0
	c.engine.Resources[id] = o
	c.engine.resourcesMu.Unlock()
	if c.run.AfterWait != nil {
		c.run.AfterWait()
	}
	return c.run.Exit, c.run.WaitFailure
}
func (c *runConnection) Remove(ctx context.Context, id string) {
	c.memoryConnection.Remove(ctx, id)
	c.engine.resourcesMu.Lock()
	_, present := c.engine.Resources[id]
	c.engine.resourcesMu.Unlock()
	if !present {
		clear(c.run.CredentialAcquired)
		c.run.CredentialAcquired = nil
	}
}
func (c *runConnection) RemoveRun(ctx context.Context, id string) {
	if c.isProxy(id) {
		c.removeProxy(ctx, id)
		return
	}
	c.run.event("remove")
	c.Remove(ctx, id)
	if _, status := c.Inspect(ctx, id); status == port.NotFound {
		clear(c.run.CredentialAcquired)
		c.run.CredentialAcquired = nil
	}
}
func (c *runConnection) Signal(ctx context.Context, id, signal string) port.Outcome {
	if c.isProxy(id) {
		return c.signalProxy(ctx, id, signal)
	}
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
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	o := c.engine.Resources[id]
	o.Running, o.Status, o.PID = false, "exited", 0
	c.engine.Resources[id] = o
	return ""
}
func (a *runAttachment) Drain(ctx context.Context) port.Outcome {
	a.run.event("drain")
	if a.run.DrainUntilCancel {
		<-ctx.Done()
	}
	select {
	case <-a.outputDone:
	case <-ctx.Done():
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
func (a *runAttachment) Close() { a.run.event("detach") }
func (a *runAttachment) Failures() <-chan port.Outcome {
	if a.run.StdinFailures != nil {
		return a.run.StdinFailures
	}
	return a.failures
}

type Workspace struct {
	AbsentProcesses map[int]bool
	Descriptor      string
	Mounts          map[int]port.MountSource
	binding         port.Binding
	Failure         port.Outcome
	Changed         bool
	Closed          bool
}

func (w *Workspace) Pin(b port.Binding) port.Outcome {
	w.Closed = w.Failure != ""
	w.binding = b
	return w.Failure
}
func (w *Workspace) Validate() port.Outcome {
	if w.Changed || w.Closed {
		return port.Refused
	}
	return ""
}
func (w *Workspace) Close() { w.Closed = true }

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
	id, f := c.CreateRun(ctx, RunIntent(b), argv)
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
