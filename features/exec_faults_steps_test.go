package features

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// The exec launch-fault, event and cleanup-fault scenarios. The fixture
// stages five distinct synthetic markers, delivers marked engine errors on
// purpose, and captures every control-state file before faults clear and
// again after fresh status and the printed stop retry, so a leak at any
// point stays provable. Deliberate stream prefixes are asserted as their
// own receipts; everything else the controller prints is diagnostics that
// must stay marker-free.
func (w *world) execFaultSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a live exec fixture with launch fault "([^"]*)" and a healthy control$`, w.execLaunchFaultFixture)
	sc.Step(`^the exec fixture stages distinct synthetic argv caller-environment API-body stdout and stderr markers$`, w.execStageMarkers)
	sc.Step(`^both exec attempts run through the local backends with deadline observations$`, w.execLaunchFaultAttempts)
	sc.Step(`^the faulty exec reports "([^"]*)" with exit 75 within its operation and startup budgets and the control exits 23$`, w.execFaultBudget)
	sc.Step(`^exec start was never retried and the faulty session is "([^"]*)"$`, w.execFaultSession)
	sc.Step(`^a late engine reply cannot start work or change the reported result$`, w.execLateFaultReply)
	sc.Step(`^every injected marked API error was delivered to the exec controller before it reported failure$`, w.execMarkedDelivered)
	sc.Step(`^the healthy exec control delivers the exact marked stdout and stderr and both frozen snapshots remain unchanged$`, w.execControlStreams)
	sc.Step(`^exec controller and recovery diagnostics and all captured control-state files contain none of the synthetic markers or raw payloads$`, w.execMarkerExclusion)
}

var execMarkerSet = []string{"slice-38-argv-marker", "slice-38-env-marker", "slice-38-api-body-marker", "slice-38-stdout-marker", "slice-38-stderr-marker"}

func (w *world) execStageMarkers() error {
	e := w.exec
	e.markers = execMarkerSet
	e.argv = append([]string{"/workspace/exec-probe"}, "slice-38-argv-marker")
	e.envExtra["SLICE_38_CALLER"] = "slice-38-env-marker"
	e.execStdout, e.execStderr = "slice-38-stdout-marker deliberate stdout prefix\n", "slice-38-stderr-marker deliberate stderr prefix\n"
	var err error
	e.frozenSnapshot, err = sessionReadRecord(w.sessionSnapshotPath())
	if err != nil || e.frozenSnapshot.Bytes == "" {
		return fmt.Errorf("missing original frozen snapshot: %v", err)
	}
	return w.execCaptureState("markers staged")
}

func (w *world) execLaunchFaultFixture(fault string) error {
	switch fault {
	case "operation lock does not become available", "create accepted but response lost",
		"create ignores cancellation past ten seconds", "start accepted but attach response lost",
		"start accepted but marked API error returned", "start ignores cancellation past ten seconds",
		"original 120-second startup budget expires before start":
	default:
		return fmt.Errorf("unknown exec launch fault %q", fault)
	}
	if err := w.execFixture(""); err != nil {
		return err
	}
	e := w.exec
	e.launchFault, e.execExit = fault, 23
	return nil
}

// execHoldOperationLock keeps the session's operation lock open through the
// whole scenario, so the launch's lock acquisition can only time out.
func (w *world) execHoldOperationLock() error {
	binary, err := w.execLeaseBinary()
	if err != nil {
		return err
	}
	ready := filepath.Join(w.dir, "slice-38-operation-ready")
	cmd := exec.Command(binary, filepath.Join(w.execSessionDir(), "operation.lock"), ready)
	cmd.Stdout, cmd.Stderr = nil, nil
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	w.exec.opLockLease = &execLease{cmd: cmd, stdin: stdin, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(w.exec.opLockLease.done) }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, readErr := os.ReadFile(ready); readErr == nil && string(data) == "ready" {
			return nil
		}
		select {
		case <-w.exec.opLockLease.done:
			return fmt.Errorf("operation lock holder exited early")
		case <-timer.C:
			return fmt.Errorf("operation lock holder never reported ready")
		case <-ticker.C:
		}
	}
}

// execLaunchFaultAttempts runs the healthy control through both backends
// with the launch fault disarmed, then the faulty attempt with it armed, so
// the control proves the fixture can still launch a healthy exec.
func (w *world) execLaunchFaultAttempts() error {
	fault := w.exec.launchFault
	w.exec.launchFault = ""
	for _, backend := range []string{"memory", "disk"} {
		if err := w.execInvoke(backend+"-control", backend, execProbeRequest{}); err != nil {
			return err
		}
	}
	w.exec.launchFault = fault
	if fault == "operation lock does not become available" {
		if err := w.execHoldOperationLock(); err != nil {
			return err
		}
	}
	for _, backend := range []string{"memory", "disk"} {
		request := execProbeRequest{Fault: fault}
		if err := w.execInvoke(backend+"-attempt", backend, request); err != nil {
			return err
		}
	}
	return w.execCaptureState("after launch fault")
}

func (w *world) execFaultBudget(rule string) error {
	for _, backend := range []string{"memory", "disk"} {
		attempt, ok := w.exec.results[backend+"-attempt"]
		if !ok {
			return fmt.Errorf("no %s attempt result was recorded", backend)
		}
		control := w.exec.results[backend+"-control"]
		if attempt.missing || control.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence\n%s", backend, w.execReport(backend+"-attempt"))
		}
		r := attempt.result()
		if r.exit != 75 || !strings.Contains(r.stderr, rule) {
			return fmt.Errorf("%s: the faulty exec did not report %s with exit 75\n%s", backend, rule, w.execReport(backend+"-attempt"))
		}
		budget := int64(11000)
		switch w.exec.launchFault {
		case "original 120-second startup budget expires before start":
			budget = 121000
		case "start accepted but attach response lost", "start accepted but marked API error returned", "start ignores cancellation past ten seconds":
			budget = 31000
		}
		if attempt.report.ElapsedMS <= 0 || attempt.report.ElapsedMS > budget {
			return fmt.Errorf("%s: the faulty exec left its budget: %d ms\n%s", backend, attempt.report.ElapsedMS, w.execReport(backend+"-attempt"))
		}
		if c := control.result(); c.exit != 23 || control.report.StartedExecs != 1 {
			return fmt.Errorf("%s: the healthy control did not start and exit 23\n%s", backend, w.execReport(backend+"-control"))
		}
	}
	return nil
}

func (w *world) execFaultSession(session string) error {
	for _, backend := range []string{"memory", "disk"} {
		attempt := w.exec.results[backend+"-attempt"]
		if attempt.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence", backend)
		}
		if backend == "disk" {
			creates, starts := 0, 0
			for _, q := range attempt.execRequests {
				if q.Method == "POST" && strings.HasPrefix(q.Path, "/containers/") && strings.HasSuffix(q.Path, "/exec") {
					creates++
				}
				if q.Method == "POST" && strings.HasPrefix(q.Path, "/exec/") && strings.HasSuffix(q.Path, "/start") {
					starts++
				}
			}
			if creates > 1 || starts > 1 {
				return fmt.Errorf("%s: exec retried its launch (%d creates, %d starts)", backend, creates, starts)
			}
		}
		if attempt.report.StartedExecs > 1 {
			return fmt.Errorf("%s: exec retried its launch", backend)
		}
	}
	running := session == "still running"
	if worker := w.session.api.local[w.exec.worker]; running != (worker != nil && object(worker["State"])["Running"] == true) {
		return fmt.Errorf("the faulty session is not %s", session)
	}
	data, err := os.ReadFile(filepath.Join(w.execSessionDir(), "record.json"))
	if err != nil {
		return err
	}
	state := "stopped"
	if running {
		state = "running"
	}
	if !strings.Contains(string(data), `"state":"`+state+`"`) {
		return fmt.Errorf("the faulty session's durable state is neither %s: %s", state, data)
	}
	return nil
}

func (w *world) execLateFaultReply() error {
	w.session.api.mu.Lock()
	before := len(w.session.api.requests)
	w.session.api.mu.Unlock()
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	w.session.api.mu.Lock()
	after := len(w.session.api.requests)
	w.session.api.mu.Unlock()
	if after != before {
		return fmt.Errorf("a late engine request resumed the refused launch")
	}
	return nil
}

func (w *world) execMarkedDelivered() error {
	wanted := 0
	switch w.exec.launchFault {
	case "start accepted but marked API error returned":
		wanted = 1
	}
	switch w.exec.cleanupFault {
	case "worker removal unavailable", "engine unavailable through teardown deadline":
		wanted = 1
	}
	w.session.api.mu.Lock()
	delivered := w.exec.markedDelivered
	w.session.api.mu.Unlock()
	if delivered < wanted {
		return fmt.Errorf("only %d of %d marked API errors reached the exec controller", delivered, wanted)
	}
	return nil
}

func (w *world) execControlStreams() error {
	for _, backend := range []string{"memory", "disk"} {
		control := w.exec.results[backend+"-control"]
		if control.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence", backend)
		}
		c := control.result()
		if c.stdout != w.exec.execStdout || c.stderr != w.exec.execStderr {
			return fmt.Errorf("%s: the control's marked streams did not arrive intact\n%s", backend, w.execReport(backend+"-control"))
		}
	}
	return w.execFrozenInputsUnchanged()
}

// The immutable frozen snapshot survives a normal exit and a teardown.
// Recovery updates the separate intent as it releases the worker.
func (w *world) execFrozenInputsUnchanged() error {
	snapshot, err := sessionReadRecord(w.sessionSnapshotPath())
	if err != nil {
		return err
	}
	if snapshot != w.exec.frozenSnapshot {
		return fmt.Errorf("the original frozen snapshot changed")
	}
	return nil
}

// execMarkerExclusion checks every captured observation: the controller's
// own diagnostics, the fresh status and stop output, and each capture of
// the persisted control state, taken before faults cleared and again after
// the retry. Deliberate stream prefixes are asserted separately; nothing
// else may carry a marker or its raw payload.
func (w *world) execMarkerExclusion() error {
	for _, key := range sortedExecResultKeys(w.exec.results) {
		b := w.exec.results[key]
		r := b.result()
		stdout, stderr := r.stdout, r.stderr
		if w.eventPrefixStreams(key) {
			stdout = strings.TrimPrefix(stdout, w.exec.execStdout)
			stderr = strings.TrimPrefix(stderr, w.exec.execStderr)
		}
		for _, marker := range w.exec.markers {
			if strings.Contains(stdout, marker) || strings.Contains(stderr, marker) {
				return fmt.Errorf("%s: controller diagnostics disclosed %s\n%s", key, marker, w.execReport(key))
			}
		}
	}
	if err := w.execRunFreshStatus(); err != nil {
		return err
	}
	text := w.exec.statusOut + w.exec.statusErr + w.stdout + w.stderr
	for _, marker := range w.exec.markers {
		if strings.Contains(text, marker) {
			return fmt.Errorf("fresh status output disclosed %s", marker)
		}
	}
	for i, capture := range w.exec.captures {
		for path, data := range capture {
			for _, marker := range w.exec.markers {
				if strings.Contains(data, marker) {
					return fmt.Errorf("control-state capture %d disclosed %s at %s", i+1, marker, path)
				}
			}
		}
	}
	return nil
}

func (w *world) execRunFreshStatus() error {
	if w.exec.statusOut != "" || w.exec.statusErr != "" || w.exec.statusExit != 0 {
		return nil
	}
	if err := w.runEnv(w.doctor.checkout, w.env(), w.bin, "status", "--json"); err != nil {
		return err
	}
	w.exec.statusExit, w.exec.statusOut, w.exec.statusErr = w.exit, w.stdout, w.stderr
	if w.exec.statusExit != 0 && w.exec.statusExit != 1 && w.exec.statusExit != 75 {
		return fmt.Errorf("fresh status failed unexpectedly\n%s", w.report())
	}
	return nil
}

// execCaptureState captures the bytes of every persisted control-state file
// under the fixture's state root, at the point the scenario names.
func (w *world) execCaptureState(point string) error {
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	capture := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Type() != 0 {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		capture[path] = string(data)
		return nil
	})
	if err != nil {
		return err
	}
	w.exec.captures = append(w.exec.captures, capture)
	return nil
}

func sortedExecResultKeys(results map[string]execBackend) []string {
	keys := make([]string, 0, len(results))
	for key := range results {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
