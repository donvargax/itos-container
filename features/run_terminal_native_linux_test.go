package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

type terminalProbeReport struct {
	TTY               [3]bool
	Initial           [2]int
	Sizes             [][2]int
	Args, Environment []string
	UID, GID          int
	Workspace         string
	Input             []byte
	Signals           []string
}

type terminalNative struct {
	f                         *terminalInvocation
	mode, input, lost, signal string
	exit                      int
	err                       error
	ready, progress           bool
	interrupted, finished     time.Time
	report                    terminalProbeReport
	journal                   []offlineRequest
}

func (w *world) terminalNativeSteps(sc *godog.ScenarioContext, fixture **terminalInvocation) {
	var n *terminalNative
	setup := func(mode string, exit int) error {
		if err := w.recoveryFixtureWithProbe("terminal", func() error {
			return w.installNativeProbe("slice-37-probe", func(path string) error { return recoveryBuildHelper(w.root, path, terminalProbeSource) })
		}); err != nil {
			return err
		}
		f := &terminalInvocation{w: w}
		*fixture = f
		n = &terminalNative{f: f, mode: mode, exit: exit}
		if err := f.bindDescriptors("terminal", "terminal"); err != nil {
			return err
		}
		w.offline.api.terminal = true
		return nil
	}
	sc.Step(`^a CI terminal-run fixture with initial size 113 by 37 and a reporting probe that exits (\d+)$`, func(exit int) error { return setup("argv", exit) })
	sc.Step(`^a CI terminal-run fixture with initial size 113 by 37 and a quiet resize-reporting probe$`, func() error { return setup("resize", 0) })
	sc.Step(`^a CI terminal-run fixture with a raw input-reporting probe$`, func() error { return setup("raw", 0) })
	sc.Step(`^a CI terminal-run fixture with a canonical signal-handling probe$`, func() error { return setup("canonical", 0) })
	sc.Step(`^a CI terminal-run fixture with a signal-handling probe that exits (\d+)$`, func(exit int) error { return setup("handle", exit) })
	sc.Step(`^a CI terminal-run fixture with a "([^"]*)" probe and "([^"]*)" terminal input$`, func(probe, input string) error {
		mode, exit := "handle", 17
		if probe == "signal-ignoring" {
			mode, exit = "ignore", 137
		}
		if err := setup(mode, exit); err != nil {
			return err
		}
		n.input = input
		return nil
	})
	sc.Step(`^the CI fixture binds stdin and stdout to (shared|distinct) terminals with saved nondefault settings$`, func(layout string) error {
		if layout == "distinct" {
			return n.f.distinctOutput()
		}
		return nil
	})
	sc.Step(`^terminal run launches the probe with an empty argument spaces and literal wrapper-looking flags$`, func() { n.run("argv", "") })
	sc.Step(`^terminal run remains quiet beyond startup and call budgets then the harness changes size to 151 by 49 and bursts through 79 by 23 to 167 by 53$`, func() { n.run("resize", "") })
	sc.Step(`^terminal run reaches the raw probe ready frame and the harness writes bytes 03 04 41 without a newline then a final release frame$`, func() { n.run("raw", "") })
	sc.Step(`^terminal run reaches the canonical probe ready frame and the harness writes byte 03 followed by a release line after the signal receipt$`, func() { n.run("canonical", "") })
	sc.Step(`^terminal run reaches the ready frame and the harness sends host signal "([^"]*)" then repeats SIGINT and SIGTERM during cleanup$`, func(signal string) { n.signal = signal; n.run("signal", "") })
	sc.Step(`^terminal run reaches the ready frame and the harness closes the (shared|stdin|stdout) terminal master while leaving the engine reachable$`, func(lost string) { n.lost = lost; n.run("disconnect", lost) })
	sc.Step(`^the probe observes all three worker descriptors as terminals and initial size 113 by 37$`, func() error {
		if err := n.observed(); err != nil {
			return err
		}
		if n.report.TTY != ([3]bool{true, true, true}) || n.report.Initial != ([2]int{113, 37}) {
			return fmt.Errorf("worker terminal report differs: %+v", n.report)
		}
		return nil
	})
	sc.Step(`^the probe observes exactly the supplied argv frozen workspace identity environment and isolation$`, func() error {
		if err := n.observed(); err != nil {
			return err
		}
		if !reflect.DeepEqual(n.report.Args, []string{"", "two words", "--tty", "--json", "--no-json", "--", "--help"}) || strconv.Itoa(n.report.UID) != w.approval.uid || strconv.Itoa(n.report.GID) != w.approval.gid || n.report.Workspace != object(w.approval.settings["manifest"])["workspaceFolder"] {
			return fmt.Errorf("literal argv or frozen identity differs: %+v", n.report)
		}
		for _, v := range n.report.Environment {
			if strings.HasPrefix(v, "COLUMNS=") || strings.HasPrefix(v, "LINES=") || strings.Contains(v, "slice-37-misleading") {
				return fmt.Errorf("controller terminal environment leaked")
			}
		}
		return w.offlineLaunch()
	})
	sc.Step(`^both worker output streams reach terminal stdout as the exact ordered raw payload without framing or a controller banner$`, func() error {
		if w.stdout != "slice-37:ready\n"+terminalPayload+terminalErrPayload || w.stderr != "" {
			return fmt.Errorf("raw merged stream differs: %s", w.report())
		}
		return nil
	})
	sc.Step(`^protected session records contain no terminal input or output payload$`, func() error {
		tree, err := runTree(filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container"))
		if err != nil {
			return err
		}
		for path, entry := range tree {
			for _, marker := range []string{"slice-37 stdout", "slice-37 stderr", "slice-37:ready", "release\\n"} {
				if strings.Contains(entry.Data, marker) {
					return fmt.Errorf("terminal payload persisted in %s", path)
				}
			}
		}
		return nil
	})
	sc.Step(`^independent inspection confirms owned worker and anchor removal while checkout edits approval and foreign resources survive$`, func() error { return n.complete() })
	sc.Step(`^the running probe reports initial size 113 by 37 then 151 by 49 and finally 167 by 53 without a restart$`, func() error {
		if err := n.observed(); err != nil {
			return err
		}
		if n.report.Initial != ([2]int{113, 37}) || !slices.Contains(n.report.Sizes, [2]int{151, 49}) || len(n.report.Sizes) == 0 || n.report.Sizes[len(n.report.Sizes)-1] != ([2]int{167, 53}) {
			return fmt.Errorf("initial/intermediate/latest sizes missing: %+v", n.report)
		}
		return nil
	})
	sc.Step(`^the engine journal resizes only the verified owned worker and no resize follows teardown$`, func() error { return n.signalsAndResizes(false) })
	sc.Step(`^a final input frame still reaches the probe and releases its normal zero exit$`, func() error {
		if !n.progress || w.exit != 0 || !bytes.HasSuffix(n.report.Input, []byte("release\n")) {
			return fmt.Errorf("quiet worker did not accept release: %s", w.report())
		}
		return nil
	})
	sc.Step(`^terminal run restores exact host settings and confirms owned cleanup without a cancellation diagnostic$`, func() error { return n.healthy() })
	sc.Step(`^the probe records bytes 03 04 41 exactly once before the release frame and remains live until that frame$`, func() error {
		if err := n.observed(); err != nil {
			return err
		}
		if !n.progress || !bytes.Equal(n.report.Input, []byte("\x03\x04Arelease\n")) {
			return fmt.Errorf("raw input was transformed or ended the run: %x", n.report.Input)
		}
		return nil
	})
	sc.Step(`^the host terminal adds no echo and terminal run sends no engine signal or cancellation diagnostic$`, func() error {
		if w.stdout != "slice-37:ready\n" || strings.Contains(w.stderr, "run.cancelled") {
			return fmt.Errorf("input echoed or cancelled host: %s", w.report())
		}
		return n.signalsAndResizes(true)
	})
	sc.Step(`^terminal run returns zero restores exact host settings and confirms owned cleanup$`, func() error { return n.healthy() })
	sc.Step(`^the probe records exactly one terminal-generated SIGINT and accepts the later release line$`, func() error {
		if err := n.observed(); err != nil {
			return err
		}
		if !n.progress || !reflect.DeepEqual(n.report.Signals, []string{"SIGINT"}) || string(n.report.Input) != "release\n" {
			return fmt.Errorf("canonical terminal SIGINT/release differs: %+v", n.report)
		}
		return nil
	})
	sc.Step(`^terminal run sends no engine signal and reports no controller cancellation$`, func() error {
		if strings.Contains(w.stderr, "run.cancelled") {
			return fmt.Errorf("terminal key cancelled controller")
		}
		return n.signalsAndResizes(true)
	})
	sc.Step(`^the probe records exactly one "([^"]*)" and terminal run returns the observed exit (\d+)$`, func(signal string, exit int) error {
		if err := n.observed(); err != nil {
			return err
		}
		if w.exit != exit || !reflect.DeepEqual(n.report.Signals, []string{signal}) {
			return fmt.Errorf("host signal/result differs: %+v; %s", n.report, w.report())
		}
		return nil
	})
	sc.Step(`^terminal run reports "run.cancelled" with the first host signal separately from the workload result$`, func() error {
		if !strings.Contains(w.stderr, "run.cancelled") || !strings.Contains(w.stderr, n.signal) || !strings.Contains(w.stderr, "workload outcome "+strconv.Itoa(n.exit)) {
			return fmt.Errorf("first cause/result diagnostic differs: %s", w.report())
		}
		return nil
	})
	sc.Step(`^terminal run restores exact host settings and finishes owned cleanup within the original teardown budget$`, func() error {
		if n.finished.Sub(n.interrupted) > 32*time.Second {
			return fmt.Errorf("teardown exceeded original total")
		}
		if err := n.f.restored(); err != nil {
			return err
		}
		return n.complete()
	})
	sc.Step(`^later signals do not repeat forwarding restart grace or change foreign resources$`, func() error { return n.ownedSignals(n.signal, false) })
	sc.Step(`^terminal run reports "run.terminal-disconnected" and returns authoritative workload exit (\d+) within bounded teardown$`, func(exit int) error {
		if err := n.observed(); err != nil {
			return err
		}
		if w.exit != exit || !strings.Contains(w.stderr, "run.terminal-disconnected") || n.finished.Sub(n.interrupted) > 32*time.Second {
			return fmt.Errorf("disconnect outcome differs: %s", w.report())
		}
		return nil
	})
	sc.Step(`^the worker receives one owned SIGTERM with escalation "([^"]*)" at the original grace deadline only while still owned and running$`, func(escalation string) error { return n.ownedSignals("SIGTERM", escalation == "one SIGKILL") })
	sc.Step(`^terminal restoration is attempted and any failure is reported separately without blocking cleanup$`, func() error {
		if n.finished.Sub(n.interrupted) > 32*time.Second {
			return fmt.Errorf("restoration blocked teardown")
		}
		if err := n.f.restored(); err != nil && !strings.Contains(w.stderr, "run.terminal-restore") {
			return fmt.Errorf("failed restoration was not reported: %v; %s", err, w.report())
		}
		return nil
	})
	sc.Step(`^every still-usable host terminal regains its own exact saved state$`, func() error {
		f := *fixture
		if f.outputSlave == nil {
			return nil
		}
		copy := *f
		if n.lost == "stdout" {
			copy.outputSlave = nil
		} else {
			copy.slave = nil
		}
		return copy.restored()
	})
}

func (n *terminalNative) readReport() bool {
	data, err := os.ReadFile(filepath.Join(n.f.w.doctor.checkout, "slice-37-report.json"))
	return err == nil && json.Unmarshal(data, &n.report) == nil
}

func (n *terminalNative) run(action, lost string) {
	f, w := n.f, n.f.w
	argv := []string{"/workspace/slice-37-probe", n.mode, strconv.Itoa(n.exit)}
	if action == "argv" {
		argv = append(argv, "", "two words", "--tty", "--json", "--no-json", "--", "--help")
	}
	w.offline.argv = argv
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, append([]string{"run", "--tty", "--"}, argv...)...)
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	for key, value := range w.doctor.env {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	for key, value := range map[string]string{"COLUMNS": "7", "LINES": "9", "TERM": "slice-37-misleading"} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	var diagnostic strings.Builder
	cmd.Stdin, cmd.Stdout, cmd.Stderr = f.input, f.output, &diagnostic
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		n.err = err
		return
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	stage := 0
	var readyAt time.Time
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
running:
	for {
		select {
		case <-done:
			break running
		case <-tick.C:
			if f.master != nil || f.outputMaster != nil {
				n.err = errors.Join(n.err, f.readAvailable())
			}
			if n.err != nil {
				cancel()
				continue
			}
			if !n.ready && bytes.Contains(f.payload, []byte("slice-37:ready\n")) {
				n.ready = true
				readyAt = time.Now()
			}
			if !n.ready {
				continue
			}
			n.readReport()
			write := func(b string) { _, err := f.master.Write([]byte(b)); n.err = errors.Join(n.err, err) }
			switch action {
			case "raw":
				if stage == 0 {
					write("\x03\x04A")
					stage++
				} else if stage == 1 && bytes.Equal(n.report.Input, []byte("\x03\x04A")) {
					n.progress = true
					write("release\n")
					stage++
				}
			case "canonical":
				if stage == 0 {
					write("\x03")
					stage++
				} else if stage == 1 && reflect.DeepEqual(n.report.Signals, []string{"SIGINT"}) {
					n.progress = true
					write("release\n")
					stage++
				}
			case "resize":
				if stage == 0 && time.Since(readyAt) >= engineQuietInterval {
					n.err = terminalResize(cmd, f.slave, 151, 49)
					stage++
				}
				if stage == 1 && slices.Contains(n.report.Sizes, [2]int{151, 49}) {
					n.err = errors.Join(terminalResize(cmd, f.slave, 79, 23), terminalResize(cmd, f.slave, 167, 53))
					stage++
				}
				if stage == 2 && slices.Contains(n.report.Sizes, [2]int{167, 53}) {
					n.progress = true
					write("release\n")
					stage++
				}
			case "signal":
				if stage == 0 {
					signal := syscall.SIGTERM
					if n.signal == "SIGINT" {
						signal = syscall.SIGINT
					}
					n.interrupted = time.Now()
					n.err = cmd.Process.Signal(signal)
					stage++
				}
				if stage == 1 && len(n.report.Signals) > 0 {
					n.err = errors.Join(cmd.Process.Signal(syscall.SIGINT), cmd.Process.Signal(syscall.SIGTERM))
					stage++
				}
			case "disconnect":
				if stage == 0 {
					if n.input == "unread byte" {
						write("A")
					}
					n.interrupted = time.Now()
					if lost == "stdout" {
						_ = f.outputMaster.Close()
						f.outputMaster = nil
						f.outputRead = f.outputSlave
					} else {
						_ = f.master.Close()
						f.master = nil
					}
					stage++
				}
			}
		}
	}
	n.finished = time.Now()
	f.timedOut = ctx.Err() != nil
	if f.master != nil || f.outputMaster != nil {
		n.err = errors.Join(n.err, f.readAvailable())
	}
	// The reporting probe may finish between polling ticks. Its complete final
	// output is authoritative for this noninteractive readiness observation.
	if action == "argv" && bytes.Contains(f.payload, []byte("slice-37:ready\n")) {
		n.ready = true
	}
	w.exit, w.stdout, w.stderr = cmd.ProcessState.ExitCode(), string(f.payload), diagnostic.String()
	n.readReport()
	w.offline.exit, w.offline.stdout, w.offline.stderr = w.exit, []byte(w.stdout), []byte(w.stderr)
	a := w.offline.api
	a.mu.Lock()
	n.journal = append([]offlineRequest{}, a.requests...)
	n.err = errors.Join(n.err, a.problem)
	a.mu.Unlock()
}

func (n *terminalNative) observed() error {
	if n.err != nil || !n.ready || n.f.timedOut {
		return fmt.Errorf("ready=%t timeout=%t error=%v; %s", n.ready, n.f.timedOut, n.err, n.f.w.report())
	}
	return nil
}

func (n *terminalNative) healthy() error {
	if err := n.observed(); err != nil {
		return err
	}
	w := n.f.w
	if w.exit != 0 || strings.Contains(w.stderr, "run.cancelled") {
		return fmt.Errorf("healthy TTY failed: %s", w.report())
	}
	if err := n.f.restored(); err != nil {
		return err
	}
	return n.complete()
}

func (n *terminalNative) complete() error {
	if err := n.observed(); err != nil {
		return err
	}
	w := n.f.w
	a := w.offline.api
	// The product's removal journal is not independent absence evidence. Refresh
	// the observer against the engine after the controller has finished, as the
	// other native run fixtures do, before judging its recorded absence.
	if err := a.observeAfter(); err != nil {
		return err
	}
	a.mu.Lock()
	created := append([]string{}, a.created...)
	absent := len(created) == 1 && a.absent[created[0]]
	a.mu.Unlock()
	if !absent {
		return fmt.Errorf("owned worker absence not independently observed")
	}
	workerRemoved, anchorRemoved, waited := false, false, false
	for _, q := range n.journal {
		if strings.HasSuffix(q.Path, "/wait") && q.Status == 200 && q.Observation["StatusCode"] == float64(w.exit) {
			waited = true
		}
		if q.Method == "DELETE" && q.Path == "/containers/"+created[0] && q.Status == 204 {
			workerRemoved = true
		}
		if q.Method == "DELETE" && q.Path == "/volumes/itos-container-anchor-"+w.session.token && q.Status == 204 {
			anchorRemoved = true
		}
	}
	if !workerRemoved || !anchorRemoved || !waited {
		return fmt.Errorf("missing observed workload exit/owned removal")
	}
	resources, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(resources, w.offline.resourcesBefore) {
		return fmt.Errorf("foreign resources changed")
	}
	data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-37-worker-edit"))
	if err != nil || string(data) != "slice-37 retained edit" {
		return fmt.Errorf("checkout edit lost")
	}
	protected, err := w.recoveryProtected()
	if err != nil {
		return err
	}
	for _, name := range []string{"report.json", "report.next", "worker-edit"} {
		delete(protected, filepath.Join(w.doctor.checkout, "slice-37-"+name))
	}
	if !reflect.DeepEqual(protected, w.recovery.before) {
		return fmt.Errorf("approval or protected fixture changed")
	}
	return nil
}

func (n *terminalNative) signalsAndResizes(noSignals bool) error {
	if err := n.observed(); err != nil {
		return err
	}
	w := n.f.w
	a := w.offline.api
	a.mu.Lock()
	ids := append([]string{}, a.created...)
	a.mu.Unlock()
	if len(ids) != 1 {
		return fmt.Errorf("expected one owned worker")
	}
	teardown := false
	for _, q := range n.journal {
		if q.Method == "DELETE" || strings.HasSuffix(q.Path, "/kill") {
			teardown = true
		}
		if strings.HasSuffix(q.Path, "/kill") && noSignals {
			return fmt.Errorf("terminal bytes caused an engine signal")
		}
		if strings.HasSuffix(q.Path, "/resize") && (q.Path != "/containers/"+ids[0]+"/resize" || teardown || q.Status != 200) {
			return fmt.Errorf("unowned or late resize")
		}
	}
	return nil
}

func (n *terminalNative) ownedSignals(first string, escalate bool) error {
	if err := n.signalsAndResizes(false); err != nil {
		return err
	}
	var signals []string
	for _, q := range n.journal {
		if strings.HasSuffix(q.Path, "/kill") {
			query, err := url.ParseQuery(q.Query)
			if err != nil {
				return err
			}
			signals = append(signals, query.Get("signal"))
			if q.State["Running"] != true {
				return fmt.Errorf("signal without fresh live ownership")
			}
			if query.Get("signal") == "SIGKILL" && (q.At.Sub(n.interrupted) < engineGraceBudget || q.At.Sub(n.interrupted) > engineGraceBudget+engineEscalationTolerance) {
				return fmt.Errorf("escalation moved original grace deadline")
			}
		}
	}
	want := []string{first}
	if escalate {
		want = append(want, "SIGKILL")
	}
	if !reflect.DeepEqual(signals, want) {
		return fmt.Errorf("signals=%v, want %v", signals, want)
	}
	return nil
}
