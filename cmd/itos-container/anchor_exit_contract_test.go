package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
	status "github.com/donvargax/itos-container/internal/slices/status/command"
)

type anchorExitObservation struct {
	Run, JSONStatus, TextStatus runExitCommand
	Record                      string
	Events                      []string
	Workers, Anchors            int
}

func TestPublicAnchorExitContract(t *testing.T) {
	mode := os.Getenv("SLICE_17_EXIT_POINT")
	if mode == "" {
		t.Skip("anchor exit feature subprocess only")
	}
	exit, err := strconv.Atoi(os.Getenv("SLICE_17_EXIT"))
	if err != nil {
		t.Fatal(err)
	}
	m := publicMemoryFixture(t)
	m.e.Exit, m.e.WaitFailure = exit, ""
	j := &runExitJournal{}
	w := &anchorExitWorkspace{Workspace: sf.MountWorkspace(m.b), engine: m.e, mode: mode, journal: j}
	m.e.AfterStart = func() {
		w.started = true
		if mode == "before inspection" {
			w.exit()
		}
	}
	m.e.After = j.add
	runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: anchorExitEngine{m.e, w}, Workspace: w, Entropy: &publicMemoryEntropy{}}
	approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
	var out, errout bytes.Buffer
	code := (runcommand.CLI{Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}).Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
	r := anchorExitObservation{Run: runExitCommand{code, out.String(), errout.String()}, Workers: len(m.e.Resources), Anchors: len(m.e.Anchors)}
	for _, f := range m.files.Sessions {
		r.Record = string(f.Data)
	}
	for _, jsonOutput := range []bool{true, false} {
		out.Reset()
		errout.Reset()
		code = (status.CLI{Service: m.service(), Presentation: presentation.UI{Out: &out, Err: &errout}, JSON: jsonOutput}).Run()
		observed := runExitCommand{code, out.String(), errout.String()}
		if jsonOutput {
			r.JSONStatus = observed
		} else {
			r.TextStatus = observed
		}
	}
	r.Events = j.events
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("SLICE_17_EXIT_RESULT=%s\n", data)
}

type anchorExitWorkspace struct {
	*sf.Workspace
	engine          *sf.RunEngine
	mode            string
	journal         *runExitJournal
	started, exited bool
	exitPending     bool
}

func (w *anchorExitWorkspace) exit() {
	w.exited = true
	for id, o := range w.engine.Resources {
		o.Running, o.Status, o.PID = false, "exited", 0
		w.engine.Resources[id] = o
	}
}

func (w *anchorExitWorkspace) HoldProcess(pid int) (sp.Process, sp.Outcome) {
	w.journal.add("pidfd-open:" + strconv.Itoa(pid))
	if w.mode == "before pidfd open with delayed engine exit" {
		w.exited, w.exitPending = true, true
		w.journal.add("pidfd-open-absent")
		return nil, sp.ProcessAbsent
	}
	if w.mode == "PID reused" {
		for id, o := range w.engine.Resources {
			o.PID = 456
			w.engine.Resources[id] = o
		}
	}
	return &anchorExitProcess{w: w}, ""
}

type anchorExitProcess struct{ w *anchorExitWorkspace }

func (p *anchorExitProcess) Exited() (bool, sp.Outcome) {
	if p.w.mode == "before stat with delayed engine exit" {
		p.w.exited, p.w.exitPending = true, true
	}
	return p.w.exited, ""
}
func (p *anchorExitProcess) Close() { p.w.journal.add("pidfd-close") }

func (w *anchorExitWorkspace) Mounted(pid int) (sp.MountSource, sp.Outcome) {
	w.journal.add("mounted-stat")
	if w.mode == "during stat with delayed engine exit" {
		w.exited, w.exitPending = true, true
		return sp.MountSource{}, sp.MountRefused
	}
	if w.mode == "between inspect and stat" {
		w.exit()
		return sp.MountSource{}, sp.MountRefused
	}
	return w.Workspace.Mounted(pid)
}

type anchorExitEngine struct {
	sp.RunEngine
	w *anchorExitWorkspace
}

func (e anchorExitEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	c, f := e.RunEngine.OpenRun(ctx, b)
	if f != "" {
		return c, f
	}
	return anchorExitConnection{c, e.w}, ""
}

type anchorExitConnection struct {
	sp.RunConnection
	w *anchorExitWorkspace
}

func (c anchorExitConnection) Inspect(ctx context.Context, id string) (sp.Observation, sp.Inspection) {
	o, s := c.RunConnection.Inspect(ctx, id)
	if c.w.started {
		c.w.journal.add("inspect:" + strconv.Itoa(o.PID))
	}
	return o, s
}

func (c anchorExitConnection) Wait(ctx context.Context, id string) (int, sp.Outcome) {
	if c.w.exitPending {
		c.w.journal.add("engine-exit-wait")
		c.w.exitPending = false
		c.w.exit()
	}
	return c.RunConnection.Wait(ctx, id)
}

func (w *anchorExitWorkspace) MountError() sp.MountSystemError {
	if strings.HasPrefix(w.mode, "before pidfd open") {
		return sp.MountSystemError{Errno: 3, Message: "no such process"}
	}
	return sp.MountSystemError{}
}
