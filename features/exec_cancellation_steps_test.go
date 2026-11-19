package features

// The exec cancellation and stop-race steps: the fixture schedules a real
// cancelled context at a port boundary, or pauses an exec transition so an
// independent stop of the same session completes first.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) execCancellationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a live exec fixture with "([^"]*)" scheduled at "([^"]*)"$`, func(signal, boundary string) error {
		if boundary != "before session lookup" && boundary != "after exec create before start" {
			return fmt.Errorf("unknown cancellation boundary %q", boundary)
		}
		if signal != "SIGINT" && signal != "SIGTERM" {
			return fmt.Errorf("unknown cancellation signal %q", signal)
		}
		if err := w.execFixture(""); err != nil {
			return err
		}
		w.exec.event, w.exec.boundary = signal, boundary
		return nil
	})
	sc.Step(`^the exec CLI attempts its probe through both local backends$`, func() error {
		return w.execEachBackends(nil)
	})
	sc.Step(`^exec reports "([^"]*)" with "([^"]*)" and exits (\d+) without an exec start$`, w.execCancelled)
	sc.Step(`^no signal or removal reaches the primary and its session remains usable$`, w.execPrimaryUntouchedByCancellation)
	sc.Step(`^a live exec fixture with a stop barrier at "([^"]*)"$`, func(boundary string) error {
		if boundary != "after admission before exec create" && boundary != "after exec create before exec start" {
			return fmt.Errorf("unknown stop barrier %q", boundary)
		}
		if err := w.execFixture(""); err != nil {
			return err
		}
		e := w.exec
		e.stopBoundary = boundary
		e.stopReady, e.stopRelease = make(chan struct{}), make(chan struct{})
		return nil
	})
	sc.Step(`^exec pauses and a separate stop completes before the exec continuation resumes through both local backends$`, w.execStopWinsRace)
	sc.Step(`^exec refuses with "([^"]*)" and exit 1 and the independent stop exits 0$`, w.execFencedByStop)
	sc.Step(`^no subsequent exec start, worker restart or state rewrite occurs after durable cleanup$`, w.execNoRestartAfterStop)
}

// Cancellation before any start may be sent leaves the owned session alone:
// the primary is neither signalled nor removed, and the durable state stays
// usable, so the next exec still finds a live owned session.
func (w *world) execCancelled(rule, signal string, exit int) error {
	for _, backend := range []string{"memory", "disk"} {
		b, ok := w.exec.results[backend]
		if !ok {
			return fmt.Errorf("no %s backend result was recorded", backend)
		}
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence\n%s", backend, w.execReport(backend))
		}
		r := b.result()
		if r.exit != exit || !strings.Contains(r.stderr, rule) || !strings.Contains(r.stderr, signal) {
			return fmt.Errorf("%s: exec did not report %s with %s and exit %d\n%s", backend, rule, signal, exit, w.execReport(backend))
		}
		if b.report.ExecStart || b.report.StartedExecs != 0 {
			return fmt.Errorf("%s: a cancelled exec started work", backend)
		}
		for _, q := range b.execRequests {
			if strings.HasPrefix(q.Path, "/exec/") && strings.HasSuffix(q.Path, "/start") {
				return fmt.Errorf("%s: a cancelled exec was started", backend)
			}
		}
	}
	return nil
}

func (w *world) execPrimaryUntouchedByCancellation() error {
	for _, backend := range []string{"memory", "disk"} {
		for _, q := range w.exec.results[backend].execRequests {
			if strings.HasSuffix(q.Path, "/kill") || q.Method == "DELETE" {
				return fmt.Errorf("%s: cancellation signalled or removed the primary: %s %s", backend, q.Method, q.Path)
			}
		}
	}
	if err := w.execPrimaryUntouched(); err != nil {
		return err
	}
	if lease := w.exec.lease; lease != nil {
		select {
		case <-lease.done:
			return fmt.Errorf("the fixture's controller lease holder died")
		default:
		}
	}
	return nil
}

// execStopWinsRace pauses each backend's exec at the fixture's barrier,
// completes an independent stop of the same session while the exec holds no
// operation lock, then releases the continuation. The memory backend applies
// its stop inside the assembly over the same port fakes; the disk backend's
// stop is this harness's own invocation of the real stop command.
func (w *world) execStopWinsRace() error {
	release := sync.OnceFunc(func() { close(w.exec.stopRelease) })
	for _, backend := range []string{"memory", "disk"} {
		w.exec.stopSignaled = false
		handle, err := w.execProbeStart(backend+"-attempt", backend, execProbeRequest{})
		if err != nil {
			return err
		}
		go func() { handle.wait <- handle.cmd.Wait() }()
		select {
		case <-w.exec.stopReady:
			if backend == "disk" {
				if err := w.execRunIndependentStop(); err != nil {
					handle.exited, handle.waitErr = true, <-handle.wait
					release()
					return err
				}
			}
			release()
			handle.exited, handle.waitErr = true, <-handle.wait
		case err := <-handle.wait:
			// A probe without the exec command never pauses; its early exit is
			// recorded as data and the behaviour assertions fail on it.
			handle.exited, handle.waitErr = true, err
		case <-time.After(20 * time.Second):
			_ = handle.cmd.Process.Kill()
			handle.exited, handle.waitErr = true, <-handle.wait
			release()
			return fmt.Errorf("%s: exec never reached its stop barrier", backend)
		}
		if err := w.execProbeWait(handle); err != nil {
			return err
		}
	}
	return nil
}

// execRunIndependentStop invokes the real stop command against the same
// recording fixture, so the pause proves the exec held no operation lock.
func (w *world) execRunIndependentStop() error {
	if err := w.runEnv(w.doctor.checkout, w.env(), w.bin, "stop", "--json", w.exec.id); err != nil {
		return err
	}
	w.exec.independentStopExit = w.exit
	return nil
}

func (w *world) execFencedByStop(rule string) error {
	for _, backend := range []string{"memory", "disk"} {
		b, ok := w.exec.results[backend+"-attempt"]
		if !ok {
			return fmt.Errorf("no %s attempt result was recorded", backend)
		}
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence\n%s", backend, w.execReport(backend+"-attempt"))
		}
		r := b.result()
		if r.exit != 1 || !strings.Contains(r.stderr, rule) {
			return fmt.Errorf("%s: the fenced exec was not refused as %s\n%s", backend, rule, w.execReport(backend+"-attempt"))
		}
		if backend == "disk" {
			if w.exec.independentStopExit != 0 {
				return fmt.Errorf("the independent stop exited %d, not 0", w.exec.independentStopExit)
			}
			continue
		}
		if b.report.StopExit != 0 {
			return fmt.Errorf("the assembly's independent stop exited %d, not 0", b.report.StopExit)
		}
	}
	return nil
}

func (w *world) execNoRestartAfterStop() error {
	data, err := os.ReadFile(filepath.Join(w.execSessionDir(), "record.json"))
	if err != nil {
		return err
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	if record["state"] != "stopped" {
		return fmt.Errorf("the durable record lost the independent stop's completion: %s", data)
	}
	for _, backend := range []string{"memory", "disk"} {
		b, ok := w.exec.results[backend+"-attempt"]
		if !ok {
			return fmt.Errorf("no %s attempt result was recorded", backend)
		}
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence", backend)
		}
		if b.report.Restarted {
			return fmt.Errorf("%s: the fenced exec restarted session resources", backend)
		}
		if backend != "disk" {
			continue
		}
		stop := -1
		for i, q := range b.execRequests {
			if q.Method == "DELETE" && q.Path == "/containers/"+w.exec.worker {
				stop = i
			}
		}
		if stop < 0 {
			return fmt.Errorf("the independent stop's removal was never recorded")
		}
		for _, q := range b.execRequests[stop+1:] {
			if q.Method == "POST" || q.Method == "DELETE" {
				return fmt.Errorf("a restart or removal followed the durable cleanup: %s %s", q.Method, q.Path)
			}
		}
	}
	return nil
}
