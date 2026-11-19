package features

// The exec event and cancellation-cleanup scenarios: a started exec meets a
// scheduled event with its exit observation kept honest, and a cancellation
// whose teardown hits a cleanup fault the printed stop retry must survive.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) execEventSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a started exec fixture with event "([^"]*)" and exit observation "([^"]*)"$`, w.execEventFixture)
	sc.Step(`^a started exec fixture with observed exit (\d+) and cancellation cleanup fault "([^"]*)"$`, w.execCleanupFaultFixture)
	sc.Step(`^the exec CLI settles the event through both local backends$`, w.execSettleEvent)
	sc.Step(`^exec cancellation completes and fresh status inspects the session through both local backends$`, w.execCancellationAndStatus)
	sc.Step(`^exec exits (\d+) with separate diagnostic "([^"]*)" and primary and exec results remain distinct$`, w.execEventResult)
	sc.Step(`^the fixture session is "([^"]*)" and no exec exit is invented$`, w.execEventSession)
	sc.Step(`^exec delivered the exact marked stdout and stderr before the event and preserved the frozen snapshot$`, w.execEventStreams)
	sc.Step(`^exec exits (\d+) and reports "([^"]*)" separately with its original session ID and exact stop retry$`, w.execCleanupReport)
	sc.Step(`^incomplete cleanup is not reported as stopped and no foreign resource is changed$`, w.execCleanupIncomplete)
	sc.Step(`^after the fault is cleared the printed stop retry confirms cleanup without substituting the exec exit for the primary result or losing checkout edits$`, w.execRetryAfterFault)
}

func (w *world) execEventFixture(event, observed string) error {
	switch event {
	case "SIGINT", "SIGTERM", "output sink failure", "attachment transport lost",
		"primary exit and exec removed before inspection":
		if observed != "23" && observed != "0" && observed != "missing" && observed != "null" {
			return fmt.Errorf("unknown exit observation %q", observed)
		}
	case "exec stopped with missing exit field":
		event, observed = "", "missing"
	case "exec stopped with null exit field":
		event, observed = "", "null"
	default:
		return fmt.Errorf("unknown exec event %q", event)
	}
	if err := w.execFixture(""); err != nil {
		return err
	}
	e := w.exec
	e.eventSchedule, e.exitObservation, e.execExit = event, observed, 23
	if observed == "0" {
		e.execExit = 0
	}
	return nil
}

func (w *world) execCleanupFaultFixture(exit int, fault string) error {
	switch fault {
	case "worker removal unavailable", "ownership replaced before signal", "engine unavailable through teardown deadline":
	default:
		return fmt.Errorf("unknown exec cleanup fault %q", fault)
	}
	if exit != 0 && exit != 23 {
		return fmt.Errorf("unknown observed exec exit %d", exit)
	}
	if err := w.execFixture(""); err != nil {
		return err
	}
	e := w.exec
	e.eventSchedule, e.exitObservation, e.execExit, e.cleanupFault = "SIGINT", strconv.Itoa(exit), exit, fault
	if fault == "ownership replaced before signal" {
		e.substituteAuthority, e.substituteBoundary = "protected intent", "teardown signal"
	}
	return nil
}

// execEventReady runs the probe and delivers the row's event to the real
// controller process once its exec is actually running: a signal for
// SIGINT and SIGTERM, a broken sink after the deliberate stdout prefix for
// the sink-failure row, and nothing for the engine-side schedules.
func (w *world) execEventReady(backend string) error {
	request := execProbeRequest{Ready: filepath.Join(w.dir, "slice-38-"+backend+"-ready")}
	request.Schedule = w.exec.eventSchedule
	if w.exec.cleanupFault != "" {
		request.CleanupFault = w.exec.cleanupFault
		request.FollowStatus = true
	}
	if w.exec.exitObservation != "" {
		request.ExitObservation = w.exec.exitObservation
	}
	if w.exec.launchFault != "" {
		request.Fault = w.exec.launchFault
	}
	sink := w.exec.eventSchedule == "output sink failure"
	handle, err := w.execProbeStartPumped(backend+"-attempt", backend, request, sink)
	if err != nil {
		return err
	}
	go func() { handle.wait <- handle.cmd.Wait() }()
	if err := w.execAwaitEvent(handle, request.Ready, sink); err != nil {
		handle.cancel()
		handle.exited, handle.waitErr = true, <-handle.wait
		return errors.Join(err, w.execProbeWait(handle))
	}
	if !handle.exited {
		handle.exited, handle.waitErr = true, <-handle.wait
	}
	if err := w.execProbeWait(handle); err != nil {
		return err
	}
	return nil
}

// execAwaitEvent waits for the controller's readiness receipt and applies
// the row's event. A probe that exits early, as one without the exec
// command does, is data for the behaviour assertions, never a setup
// failure; the event is simply never delivered.
func (w *world) execAwaitEvent(handle *execProbeHandle, ready string, sink bool) error {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, readErr := os.ReadFile(ready)
		if readErr == nil && string(data) == "ready" {
			break
		}
		select {
		case err := <-handle.wait:
			handle.exited, handle.waitErr = true, err
			return nil
		case <-timer.C:
			return fmt.Errorf("%s: exec never reported readiness for its event", handle.backend)
		case <-ticker.C:
		}
	}
	switch w.exec.eventSchedule {
	case "SIGINT", "SIGTERM":
		signal := syscall.SIGINT
		if w.exec.eventSchedule == "SIGTERM" {
			signal = syscall.SIGTERM
		}
		if err := handle.cmd.Process.Signal(signal); err != nil {
			return err
		}
	case "output sink failure":
		// The deliberate stdout prefix must reach the controller's sink
		// before the sink breaks: watch what the probe actually forwarded,
		// then close the read end under it.
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for !strings.Contains(handle.pumped.String(), "slice-38-stdout-marker") {
			select {
			case err := <-handle.wait:
				handle.exited, handle.waitErr = true, err
				return nil
			case <-timer.C:
				return fmt.Errorf("%s: the deliberate stdout prefix never arrived", handle.backend)
			case <-ticker.C:
			}
		}
		if handle.stdoutReader != nil {
			if err := handle.stdoutReader.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *world) execSettleEvent() error {
	for _, backend := range []string{"memory", "disk"} {
		if err := w.execEventReady(backend); err != nil {
			return err
		}
	}
	return w.execCaptureState("after event")
}

func (w *world) execCancellationAndStatus() error {
	for _, backend := range []string{"memory", "disk"} {
		if err := w.execEventReady(backend); err != nil {
			return err
		}
		b := w.exec.results[backend+"-attempt"]
		if b.missing {
			continue
		}
		if backend == "disk" {
			if err := w.runEnv(w.doctor.checkout, w.env(), w.bin, "status", "--json", w.exec.id); err != nil {
				return err
			}
			w.exec.statusExit, w.exec.statusOut, w.exec.statusErr = w.exit, w.stdout, w.stderr
		}
	}
	return w.execCaptureState("after cancellation and status")
}

func (w *world) execEventResult(exit int, rule string) error {
	for _, backend := range []string{"memory", "disk"} {
		b, ok := w.exec.results[backend+"-attempt"]
		if !ok {
			return fmt.Errorf("no %s attempt result was recorded", backend)
		}
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence\n%s", backend, w.execReport(backend+"-attempt"))
		}
		r := b.result()
		if r.exit != exit || !strings.Contains(r.stderr, rule) {
			return fmt.Errorf("%s: exec did not exit %d with %s\n%s", backend, exit, rule, w.execReport(backend+"-attempt"))
		}
	}
	data, err := os.ReadFile(filepath.Join(w.execSessionDir(), "record.json"))
	if err != nil {
		return err
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	if record["workload_exit"] != nil {
		return fmt.Errorf("an exec exit was substituted for the primary result: %s", data)
	}
	return nil
}

func (w *world) execEventSession(session string) error {
	data, err := os.ReadFile(filepath.Join(w.execSessionDir(), "record.json"))
	if err != nil {
		return err
	}
	state := "stopped"
	if session == "still running" {
		state = "running"
	}
	if !strings.Contains(string(data), `"state":"`+state+`"`) {
		return fmt.Errorf("the session is not %s: %s", session, data)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	if record["workload_exit"] != nil {
		return fmt.Errorf("an exec exit was invented for the primary: %s", data)
	}
	if session == "still running" {
		if worker := w.session.api.local[w.exec.worker]; worker == nil || object(worker["State"])["Running"] != true {
			return fmt.Errorf("a still-running session lost its primary")
		}
	}
	return nil
}

func (w *world) execEventStreams() error {
	for _, backend := range []string{"memory", "disk"} {
		b := w.exec.results[backend+"-attempt"]
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence", backend)
		}
		r := b.result()
		if w.exec.eventSchedule == "output sink failure" {
			if !strings.Contains(r.stdout, "slice-38-stdout-marker") {
				return fmt.Errorf("%s: the deliberate stdout prefix never reached the broken sink\n%s", backend, w.execReport(backend+"-attempt"))
			}
		} else if r.stdout != w.exec.execStdout {
			return fmt.Errorf("%s: the marked stdout prefix did not arrive intact\n%s", backend, w.execReport(backend+"-attempt"))
		}
		if r.stderr != w.exec.execStderr {
			return fmt.Errorf("%s: the marked stderr prefix did not arrive intact\n%s", backend, w.execReport(backend+"-attempt"))
		}
	}
	return w.execFrozenInputsUnchanged()
}

func (w *world) execCleanupReport(exit int, rule string) error {
	for _, backend := range []string{"memory", "disk"} {
		b, ok := w.exec.results[backend+"-attempt"]
		if !ok {
			return fmt.Errorf("no %s attempt result was recorded", backend)
		}
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence\n%s", backend, w.execReport(backend+"-attempt"))
		}
		r := b.result()
		if r.exit != exit || !strings.Contains(r.stderr, rule) {
			return fmt.Errorf("%s: exec did not exit %d with %s\n%s", backend, exit, rule, w.execReport(backend+"-attempt"))
		}
		if !strings.Contains(r.stderr, "itos container stop "+w.exec.id) {
			return fmt.Errorf("%s: the refusal lost its original session ID and stop retry\n%s", backend, w.execReport(backend+"-attempt"))
		}
		if backend == "disk" {
			if w.exec.statusExit == 0 && !strings.Contains(w.exec.statusOut, w.exec.id) {
				return fmt.Errorf("fresh status lost the session identity")
			}
		}
	}
	return nil
}

func (w *world) execCleanupIncomplete() error {
	if w.exec.cleanupFault != "ownership replaced before signal" {
		data, err := os.ReadFile(filepath.Join(w.execSessionDir(), "record.json"))
		if err != nil {
			return err
		}
		if strings.Contains(string(data), `"state":"stopped"`) {
			return fmt.Errorf("incomplete cleanup was reported as stopped: %s", data)
		}
	}
	if worker := w.session.api.local[w.exec.worker]; worker == nil || object(worker["State"])["Running"] != true {
		return fmt.Errorf("a cleanup fault changed the primary worker")
	}
	resourcesAfter := map[string]string{}
	for id, c := range w.session.api.local {
		resourcesAfter[id] = string(approvalJSON(c))
	}
	if !reflect.DeepEqual(w.exec.resourcesBefore, resourcesAfter) {
		return fmt.Errorf("a cleanup fault changed a foreign resource")
	}
	return nil
}

func (w *world) execRetryAfterFault() error {
	attempt := w.exec.results["disk-attempt"]
	if attempt.missing {
		return fmt.Errorf("disk backend published no exec behaviour evidence")
	}
	before, err := os.ReadFile(filepath.Join(w.execSessionDir(), "record.json"))
	if err != nil {
		return err
	}
	w.session.api.mu.Lock()
	w.exec.faultCleared = true
	w.session.api.mu.Unlock()
	defer func() {
		_, _ = os.ReadFile(filepath.Join(w.doctor.checkout, "edit.txt"))
	}()
	if err := w.execCaptureState("before retry"); err != nil {
		return err
	}
	if err := w.runEnv(w.doctor.checkout, w.env(), w.bin, "stop", "--json", w.exec.id); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("the printed stop retry did not confirm cleanup\n%s", w.report())
	}
	data, err := os.ReadFile(filepath.Join(w.execSessionDir(), "record.json"))
	if err != nil {
		return err
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	if record["state"] != "stopped" {
		return fmt.Errorf("the retry did not durably complete cleanup: %s", data)
	}
	if record["workload_exit"] != nil {
		return fmt.Errorf("the retry substituted an exec exit for the primary result: %s", data)
	}
	if string(data) == string(before) && w.exec.cleanupFault != "ownership replaced before signal" {
		return fmt.Errorf("the retry rewrote nothing")
	}
	if _, err := os.Stat(filepath.Join(w.doctor.checkout, "edit.txt")); err != nil {
		return fmt.Errorf("the retry lost the checkout edits")
	}
	return w.execCaptureState("after retry")
}

// eventPrefixStreams is whether a result's channels carry the deliberate
// stream prefixes that belong to the workload, not the controller.
func (w *world) eventPrefixStreams(key string) bool {
	if w.exec.eventSchedule == "" && w.exec.launchFault == "" && w.exec.cleanupFault == "" {
		return false
	}
	if w.exec.cleanupFault != "" || w.exec.eventSchedule != "" {
		return strings.HasSuffix(key, "-attempt")
	}
	return strings.HasSuffix(key, "-control") && w.exec.launchFault != ""
}
