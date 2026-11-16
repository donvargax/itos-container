package porttest

import (
	"bytes"
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	"time"
)

func (c *runConnection) SetTerminal(size terminal.Dimensions) port.Outcome {
	if !size.Valid() {
		return port.TerminalUnavailable
	}
	c.terminal = size
	return ""
}
func (c *runConnection) Resize(ctx context.Context, id string, size terminal.Dimensions) port.Outcome {
	if !size.Valid() {
		return port.TerminalUnavailable
	}
	if ctx.Err() != nil {
		return port.TerminalResize
	}
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	if _, ok := c.engine.Resources[id]; !ok {
		return port.TerminalResize
	}
	c.terminal = size
	return ""
}

// TerminalEngineContract supplies identical bytes and initial dimensions to the
// fake and HTTP adapter. The fixture records actual wire/state independently.
func TerminalEngineContract(t Reporter, open func() (port.RunEngine, port.Binding)) {
	t.Helper()
	e, b := open()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, failure := e.OpenRun(ctx, b)
	if failure != "" {
		t.Errorf("open: %s", failure)
		return
	}
	defer c.Close()
	tc, ok := c.(port.TerminalConnection)
	if !ok {
		t.Errorf("missing terminal capability")
		return
	}
	if f := tc.SetTerminal(terminal.Dimensions{}); f != port.TerminalUnavailable {
		t.Errorf("invalid size: %s", f)
	}
	if f := tc.Resize(ctx, "uncreated", terminal.Dimensions{}); f != port.TerminalUnavailable {
		t.Errorf("invalid resize: %s", f)
	}
	if f := tc.SetTerminal(terminal.Dimensions{Columns: 113, Rows: 37}); f != "" {
		t.Errorf("initial size: %s", f)
	}
	argv := []string{"probe", "", "--tty"}
	id, failure := c.CreateRun(ctx, RunIntent(b), argv)
	if failure != "" {
		t.Errorf("create: %s", failure)
		return
	}
	if f := c.VerifyRun(ctx, id, argv); f != "" {
		t.Errorf("verify tty: %s", f)
		return
	}
	var out, errout bytes.Buffer
	a, failure := c.Attach(ctx, id, port.Streams{Out: &out, Err: &errout})
	if failure != "" {
		t.Errorf("attach: %s", failure)
		return
	}
	defer a.Close()
	if f := c.Start(ctx, id); f != "" {
		t.Errorf("start: %s", f)
	}
	if f := tc.Resize(ctx, id, terminal.Dimensions{Columns: 167, Rows: 53}); f != "" {
		t.Errorf("resize: %s", f)
	}
	ended, end := context.WithCancel(ctx)
	end()
	if f := tc.Resize(ended, id, terminal.Dimensions{Columns: 167, Rows: 53}); f == "" {
		t.Errorf("cancelled resize succeeded")
	}
	if exit, f := c.Wait(ctx, id); exit != 7 || f != "" {
		t.Errorf("wait: %d %s", exit, f)
	}
	if f := a.Drain(ctx); f != "" {
		t.Errorf("drain: %s", f)
	}
	if out.String() != "stdout\x00\xffstderr\x00\xfe" || errout.Len() != 0 {
		t.Errorf("raw terminal output: %q stderr %q", out.String(), errout.String())
	}
}
