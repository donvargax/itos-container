package features

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/cucumber/godog"
	"golang.org/x/sys/unix"
)

func (w *world) terminalLocalSteps(sc *godog.ScenarioContext, fixture **terminalInvocation) {
	sc.Step(`^the fixture runs with (stdin|stdout) addressed through the controlling-terminal alias$`, func(endpoint string) error {
		f := *fixture
		f.local, f.alias = true, endpoint
		w.doctor.engine.offline.terminal = newTerminalEngine()
		return f.invoke("run --tty -- fixture-workload", "0")
	})
	sc.Step(`^the fixture delivers a final raw output frame after workload exit during owned removal$`, func() error {
		f := *fixture
		f.local = true
		e := newTerminalEngine()
		e.tailStart, e.tailRead = make(chan struct{}), make(chan struct{})
		w.doctor.engine.offline.terminal = e
		read := false
		f.action = func(*exec.Cmd) {
			if !read && strings.Contains(string(f.payload), "slice-37 tail") {
				read = true
				close(e.tailRead)
			}
		}
		return f.invoke("run --tty -- fixture-workload", "0")
	})
	sc.Step(`^terminal stdout contains the exact initial and final raw frames$`, func() error {
		if (*fixture).timedOut || w.stdout != terminalPayload+terminalErrPayload+terminalTailPayload {
			return fmt.Errorf("final raw frame differs: %s", w.report())
		}
		return nil
	})
	sc.Step(`^a local terminal-run fixture with (shared|distinct) terminal stdin and stdout$`, func(layout string) error {
		f, err := w.terminalInvocationFixture("terminal", "terminal")
		*fixture = f
		if err != nil {
			return err
		}
		if layout == "distinct" {
			return f.distinctOutput()
		}
		return nil
	})
	sc.Step(`^the fixture invokes plain run and terminal run with workload arguments "([^"]*)"$`, func(args string) error {
		f := *fixture
		f.local = true
		w.doctor.engine.offline.terminal = newTerminalEngine()
		if err := f.invoke("run -- "+args, "0"); err != nil {
			return err
		}
		w.doctor.engine.offline.terminal = newTerminalEngine()
		return f.invoke("run --tty -- "+args, "0")
	})
	sc.Step(`^only the terminal invocation creates and verifies a TTY worker with initial size (\d+) by (\d+)$`, func(columns, rows int) error {
		f := *fixture
		a := w.doctor.engine.offline
		a.mu.Lock()
		defer a.mu.Unlock()
		if len(f.attempts) != 2 || len(a.created) != 2 {
			return fmt.Errorf("expected two successful workers: %s", w.report())
		}
		for i, id := range slices.Sorted(func(yield func(string) bool) {
			for id := range a.created {
				if !yield(id) {
					return
				}
			}
		}) {
			worker := a.created[id]
			wantTTY := i == 1
			if object(worker["Config"])["Tty"] != wantTTY {
				return fmt.Errorf("worker %s TTY selection differs", id)
			}
			if wantTTY && !reflect.DeepEqual(object(worker["HostConfig"])["ConsoleSize"], []any{float64(rows), float64(columns)}) {
				return fmt.Errorf("initial console size differs: %v", worker["HostConfig"])
			}
			inspections := 0
			for _, r := range f.attempts[i].requests {
				if strings.HasSuffix(r.Path, "/"+id+"/json") {
					inspections++
				}
			}
			if inspections < 2 || !a.started[id] {
				return fmt.Errorf("worker was not verified before start")
			}
		}
		return nil
	})
	sc.Step(`^both invocations deliver exactly the literal workload arguments$`, func() error {
		a := w.doctor.engine.offline
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.problem != nil {
			return a.problem
		}
		if len(a.created) != 2 {
			return fmt.Errorf("literal argv was not delivered twice")
		}
		return nil
	})
	sc.Step(`^the plain invocation keeps separate output streams and never changes terminal settings or resizes a worker$`, func() error {
		f := *fixture
		if len(f.attempts) != 2 {
			return fmt.Errorf("missing plain/TTY invocations")
		}
		p := f.attempts[0]
		if p.exit != 0 || p.stdout != strings.ReplaceAll(terminalPayload, "\n", "\r\n") || p.stderr != terminalErrPayload || !p.statesRestored || p.settingsChanged {
			return fmt.Errorf("plain streams or settings differ: %+v", p)
		}
		for _, r := range p.requests {
			if strings.Contains(r.Path, "/resize") {
				return fmt.Errorf("plain run resized a worker")
			}
		}
		return nil
	})
	sc.Step(`^the terminal invocation uses raw attachment output on stdout and separate controller diagnostics on stderr$`, func() error {
		if w.exit != 0 || w.stdout != terminalPayload+terminalErrPayload || w.stderr != "" {
			return fmt.Errorf("TTY raw output differs: %s", w.report())
		}
		return nil
	})
	sc.Step(`^both invocations restore saved host settings and confirm only their own resources absent$`, func() error {
		for _, a := range (*fixture).attempts {
			if !a.statesRestored || a.exit != 0 {
				return fmt.Errorf("terminal not restored after a successful run: %+v", a)
			}
		}
		return w.localOfflineOwned()
	})
	sc.Step(`^both invocations preserve approval checkout and outside sentinels except for their owned session state$`, func() error { return w.runSameTree(true) })
	sc.Step(`^the fixture terminal has (\d+) columns and (\d+) rows$`, func(columns, rows int) error {
		return pty.Setsize((*fixture).slave, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
	})
	sc.Step(`^the terminal fixture invokes run with tty and retries after setting size 113 by 37$`, func() error {
		f := *fixture
		f.local = true
		w.doctor.engine.offline.terminal = newTerminalEngine()
		if err := f.invoke("run --tty -- fixture-workload", "0"); err != nil {
			return err
		}
		if err := pty.Setsize(f.slave, &pty.Winsize{Cols: 113, Rows: 37}); err != nil {
			return err
		}
		return f.invoke("run --tty -- fixture-workload", "0")
	})
	sc.Step(`^the first terminal run reports "([^"]*)" with exit (\d+) before any engine connection or state write$`, func(rule string, code int) error {
		f := *fixture
		if f.contract != nil {
			return f.contract.firstRefusal(rule, code)
		}
		if len(f.attempts) != 2 {
			return fmt.Errorf("missing retry")
		}
		a := f.attempts[0]
		if a.exit != code || !strings.Contains(a.stderr, rule) || a.stdout != "" || a.connections != 0 || a.stateError != nil {
			return fmt.Errorf("first acquisition refusal differs: %+v", a)
		}
		return nil
	})
	sc.Step(`^the retry starts with the measured size 113 by 37 and exits zero after owned cleanup$`, func() error {
		f := *fixture
		if w.exit != 0 || len(f.attempts) != 2 {
			return fmt.Errorf("retry failed: %s", w.report())
		}
		if err := f.initialSize(113, 37); err != nil {
			return err
		}
		return w.localOfflineOwned()
	})
	sc.Step(`^both attempts leave the exact original terminal settings and approval intact$`, func() error {
		for _, a := range (*fixture).attempts {
			if !a.statesRestored {
				return fmt.Errorf("attempt did not restore its terminal")
			}
		}
		return w.runSameTree(true)
	})
	sc.Step(`^the fixture stdin has size 113 by 37 and stdout has size (\d+) by (\d+)$`, func(columns, rows int) error {
		return pty.Setsize((*fixture).outputSlave, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
	})
	var control, progress bool
	var actionError error
	sc.Step(`^the fixture measures stdout newline translation then runs a raw terminal payload with exit (\d+) while resizing stdout then stdin$`, func(exit int) error {
		f := *fixture
		f.local = true
		if _, err := f.outputSlave.Write([]byte("\n")); err != nil {
			return err
		}
		if err := f.readAvailable(); err != nil {
			return err
		}
		control = string(f.payload) == "\r\n"
		e := newTerminalEngine()
		e.hold, e.exit = true, exit
		w.doctor.engine.offline.terminal = e
		w.doctor.cleanup = append(w.doctor.cleanup, e.finish)
		stage := 0
		var changed time.Time
		f.action = func(cmd *exec.Cmd) {
			if actionError != nil {
				return
			}
			switch stage {
			case 0:
				select {
				case <-e.ready:
				default:
					return
				}
				actionError = terminalResize(cmd, f.outputSlave, 151, 49)
				changed = time.Now()
				stage++
			case 1:
				if time.Since(changed) < 30*time.Millisecond {
					return
				}
				_, actionError = f.master.Write([]byte("abc"))
				stage++
			case 2:
				if !strings.Contains(string(f.payload), "progress") {
					return
				}
				progress = true
				actionError = terminalResize(cmd, f.slave, 167, 53)
				stage++
			case 3:
				a := w.doctor.engine.offline
				a.mu.Lock()
				resized := len(e.resizes) > 0
				a.mu.Unlock()
				if resized {
					e.finish()
					stage++
				}
			}
		}
		return f.invoke("run --tty -- fixture-workload", "0")
	})
	sc.Step(`^the pre-run stdout control converts LF to CRLF and terminal stdout exactly matches the raw engine payload$`, func() error {
		if actionError != nil || !control || w.stdout != terminalPayload+terminalErrPayload+"progress" {
			return fmt.Errorf("control=%t action=%v raw payload differs: %s", control, actionError, w.report())
		}
		return nil
	})
	sc.Step(`^terminal run creates a TTY worker at stdin size 113 by 37 and resizes it only to stdin size 167 by 53$`, func() error {
		if err := (*fixture).initialSize(113, 37); err != nil {
			return err
		}
		a := w.doctor.engine.offline
		a.mu.Lock()
		defer a.mu.Unlock()
		if len(a.created) != 1 || len(a.terminal.resizes) != 1 {
			return fmt.Errorf("resize journal differs: %v", a.terminal.resizes)
		}
		for id := range a.created {
			if a.terminal.resizes[0] != id+":167x53" {
				return fmt.Errorf("resize used another terminal/worker: %v", a.terminal.resizes)
			}
		}
		return nil
	})
	sc.Step(`^the stdout-only size change to 151 by 49 leaves the run live for a later input and progress exchange$`, func() error {
		if !progress {
			return fmt.Errorf("no progress after stdout resize: %s", w.report())
		}
		return nil
	})
	sc.Step(`^terminal run returns the observed exit (\d+) with exact restoration of each host terminal's own saved state$`, func(exit int) error {
		if w.exit != exit || (*fixture).timedOut {
			return fmt.Errorf("workload result lost: %s", w.report())
		}
		return (*fixture).restored()
	})
	sc.Step(`^both caller terminal handles remain usable and only owned resources are confirmed absent$`, func() error {
		if err := (*fixture).restored(); err != nil {
			return err
		}
		return w.localOfflineOwned()
	})
	sc.Step(`^terminal run preserves approval checkout and outside sentinels$`, func() error { return w.runSameTree(true) })
	sc.Step(`^the terminal-run fixture schedules "([^"]*)"$`, func(fault string) error {
		f := *fixture
		f.local = true
		e := newTerminalEngine()
		e.fault = fault
		w.doctor.engine.offline.terminal = e
		if fault == "missing approval" {
			if err := os.Remove(w.approval.record); err != nil {
				return err
			}
			return w.runCheckpoint()
		}
		if fault == "host SIGTERM during preflight" {
			w.doctor.engine.paused = true
			f.action = func(cmd *exec.Cmd) {
				select {
				case <-w.doctor.engine.reached:
					_ = cmd.Process.Signal(syscall.SIGTERM)
					// Keep the image reply withheld until the controller returns;
					// release cannot race signal delivery into a later startup phase.
					f.action = nil
				default:
				}
			}
		}
		return nil
	})
	sc.Step(`^terminal run reports "([^"]*)" with exit (\d+) and no later startup phase$`, func(rule string, exit int) error {
		f := *fixture
		if err := f.refusal(rule, exit); err != nil {
			return err
		}
		a := w.doctor.engine.offline
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.terminal.fault != "start accepted but its reply times out" && len(a.started) != 0 {
			return fmt.Errorf("refused startup reached start")
		}
		return nil
	})
	sc.Step(`^terminal run restores each host terminal's own exact saved state and ends all terminal observers$`, func() error { return (*fixture).restored() })
	sc.Step(`^accepted resources are reconciled only under the original ownership with incomplete recovery retained when necessary$`, func() error {
		a := w.doctor.engine.offline
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.problem != nil {
			return a.problem
		}
		if len(a.resources) != 0 || len(a.volumes) != 0 {
			return fmt.Errorf("accepted resources left without cleanup")
		}
		for id := range a.created {
			if !a.removed[id] {
				return fmt.Errorf("accepted worker %s not reconciled", id)
			}
		}
		return nil
	})
}

func (f *terminalInvocation) distinctOutput() error {
	master, slave, err := pty.Open()
	if err != nil {
		return err
	}
	f.w.doctor.cleanup = append(f.w.doctor.cleanup, func() { _ = master.Close(); _ = slave.Close() })
	f.outputMaster, f.outputSlave, f.output = master, slave, slave
	if err := pty.Setsize(slave, &pty.Winsize{Cols: 79, Rows: 23}); err != nil {
		return err
	}
	state, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}
	state.Cc[unix.VEOF] = 11
	state.Oflag = unix.OPOST | unix.ONLCR
	if err := unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, state); err != nil {
		return err
	}
	f.outputSaved = state
	return nil
}

func (f *terminalInvocation) initialSize(columns, rows int) error {
	a := f.w.doctor.engine.offline
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.created) != 1 {
		return fmt.Errorf("expected one worker: %s", f.w.report())
	}
	for _, worker := range a.created {
		if object(worker["Config"])["Tty"] != true || !reflect.DeepEqual(object(worker["HostConfig"])["ConsoleSize"], []any{float64(rows), float64(columns)}) {
			return fmt.Errorf("TTY/initial dimensions differ: %v", worker)
		}
	}
	return nil
}
