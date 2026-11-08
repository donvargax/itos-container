package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

// These fixtures are selected only by Linux scenarios. Keep the real child
// unreaped until the engine wait, independently of the product's pidfd probe.
type anchorExitChild struct {
	command   *exec.Cmd
	input     io.WriteCloser
	workspace disk.Workspace
	t         *testing.T
	finished  bool
}

func newAnchorExitChild(t *testing.T, exit int) *anchorExitChild {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, "/bin/sh", "-c", `read ignored; exit "$1"`, "worker", strconv.Itoa(exit))
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = command.Process.Kill(); _ = command.Wait() })
	return &anchorExitChild{command: command, input: input, t: t}
}

func (c *anchorExitChild) finish(w *anchorExitWorkspace) sp.Outcome {
	if c.finished {
		return ""
	}
	if err := c.input.Close(); err != nil {
		c.t.Error(err)
		return sp.MountRefused
	}
	path := "/proc/" + strconv.Itoa(c.command.Process.Pid) + "/stat"
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err != nil {
			c.t.Error(err)
			return sp.MountRefused
		}
		if end := strings.LastIndex(string(data), ") "); end >= 0 && strings.HasPrefix(string(data)[end+2:], "Z ") {
			c.finished, w.exitPending = true, true
			w.journal.add("child-unreaped")
			return ""
		}
		time.Sleep(time.Millisecond)
	}
	c.t.Error("child never reached unreaped exit")
	return sp.MountRefused
}

func (c *anchorExitChild) reap() int {
	_ = c.command.Wait()
	return c.command.ProcessState.ExitCode()
}

func (c *anchorExitChild) hold(w *anchorExitWorkspace, pid int) (sp.Process, sp.Outcome) {
	p, f := c.workspace.HoldProcess(pid)
	if p == nil {
		return nil, f
	}
	return &anchorExitChildProcess{Process: p, w: w}, f
}

type anchorExitChildProcess struct {
	sp.Process
	w *anchorExitWorkspace
}

func (p *anchorExitChildProcess) Exited() (bool, sp.Outcome) {
	if p.w.mode == "unreaped before stat" {
		if f := p.w.child.finish(p.w); f != "" {
			return false, f
		}
	}
	exited, failure := p.Process.Exited()
	if strings.HasPrefix(p.w.mode, "root changed ") && !exited && failure == "" {
		p.w.journal.add("held-process-live")
	}
	return exited, failure
}

func (p *anchorExitChildProcess) Close() {
	p.Process.Close()
	p.w.journal.add("pidfd-close")
}
