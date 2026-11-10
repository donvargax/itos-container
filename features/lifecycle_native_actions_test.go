package features

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (w *world) lifecycleSignalAction(signal string, repeat bool) {
	l := w.lifecycle
	w.lifecycleStart()
	if !w.lifecycleReadyAction(func() { w.lifecycleSignal(signal) }) {
		w.lifecycleAbortEarly()
		return
	}
	if repeat {
		l.aliveBeforeGrace = lifecycleRepeatedSignals(l.run, l.signalled, w.lifecycleSignal, func() bool {
			return w.recoveryNativePresent(true)
		})
	}
	w.lifecycleCollect()
	w.lifecycleFreshRecovery(false)
}

// The last repeated signal goes this long after cancellation. A grace timer it
// restarted fires no earlier than this plus the grace, which lifecycleSignals
// refuses a SIGKILL at.
const lifecycleLastRepeat = 4 * engineGraceBudget / 5

func lifecycleRepeatedSignals(p *recoveryProcess, cancelled time.Time, signal func(string), present func() bool) bool {
	// Inspect immediately after cancellation, while repeats keep their own
	// schedule. The independent observer already bounds its request to 5 s.
	// Its reply may arrive after grace: a running observation of this same,
	// never-restarted primary still proves it survived the earlier interval.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, item := range []struct {
			after  time.Duration
			signal string
		}{{engineGraceBudget / 5, "SIGINT"}, {lifecycleLastRepeat, "SIGTERM"}} {
			if !lifecycleUntil(p, cancelled.Add(item.after)) {
				break
			}
			signal(item.signal)
		}
	}()
	alive := present()
	<-done
	return alive
}

func lifecycleUntil(p *recoveryProcess, at time.Time) bool {
	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-p.done:
		return false
	case <-timer.C:
		return true
	}
}

func lifecycleFullPipe(file *os.File) bool {
	capacity, available, err, readErr := lifecyclePipeSize(file)
	return err == nil && readErr == nil && capacity > 0 && available == capacity
}

// Drain only the pre-release prelude. In particular, a short ready frame can
// occupy a pipe page without filling it; that stranded page makes byte count
// equality miss real backpressure. Preserve these bytes and stop reading before
// releasing workload output, so no reader can help cancellation or teardown.
func lifecyclePipePrelude(file *os.File, stdout bool) ([]byte, error) {
	if err := file.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, err
	}
	defer func() { _ = file.SetReadDeadline(time.Time{}) }()
	_, size, err, readErr := lifecyclePipeSize(file)
	if err != nil || readErr != nil {
		return nil, errors.Join(err, readErr)
	}
	ready := []byte("slice-12:ready\n")
	if stdout {
		size = len(ready)
	}
	data := make([]byte, size)
	_, err = io.ReadFull(file, data)
	if err == nil && stdout && !bytes.Equal(data, ready) {
		err = fmt.Errorf("unexpected controller readiness frame: %q", data)
	}
	return data, err
}

func (w *world) lifecycleCleanupSignals() {
	l := w.lifecycle
	w.lifecycleStart()
	l.barrierReached = recoveryBarrier(l.fault.ready, l.run)
	if !l.barrierReached {
		w.lifecycleAbortEarly()
		return
	}
	a := w.offline.api
	a.mu.Lock()
	for _, e := range a.requests {
		if strings.HasSuffix(e.Path, "/wait") && e.Status == 200 && e.Observation["StatusCode"] == float64(17) {
			l.exitObserved = e.Replied
		}
	}
	a.mu.Unlock()
	w.lifecycleSignal("SIGINT")
	if lifecycleUntil(l.run, time.Now().Add(100*time.Millisecond)) {
		w.lifecycleSignal("SIGTERM")
	}
	// Both signals have now overlapped the pending removal. Return the
	// injected unavailable response promptly; keep rejecting every removal
	// until run returns and the later explicit stop restores the transport.
	l.fault.releaseBarrier()
	w.lifecycleCollect()
	l.fault.restore()
	w.lifecycleFreshRecovery(true)
}

func (w *world) lifecycleHeartbeat() int {
	data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-12-heartbeat"))
	value, parseErr := strconv.Atoi(string(data))
	if err != nil || parseErr != nil {
		return 0
	}
	return value
}

func (w *world) lifecycleDisconnect() {
	l := w.lifecycle
	if err := w.lifecycleControl(false); err != nil {
		l.err = err
		return
	}
	w.lifecycleStart()
	if !w.lifecycleReadyAction(w.lifecycleDisconnectReady) {
		w.lifecycleAbortEarly()
		return
	}
	if l.err != nil {
		w.lifecycleAbortEarly()
		return
	}
	w.lifecycleCollect()
	l.later = w.lifecycleHeartbeat()
	l.survivedLoss = w.recoveryNativePresent(true)
	var err error
	l.lostBoundary, err = w.offline.api.inspect(w.recovery.worker)
	l.err = errors.Join(l.err, err)
	l.fault.restore()
	w.lifecycleFreshRecovery(true)
}

func (w *world) lifecycleDisconnectReady() {
	l := w.lifecycle
	// An acknowledged, still-pending wait proves a healthy same-path transport
	// before we remove observation. Closing only the proxy's upgraded handles
	// also makes stream loss visible without touching the daemon or observer.
	if !w.lifecycleAwait(5*time.Second, func() bool {
		a := w.offline.api
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, e := range a.requests {
			if strings.HasSuffix(e.Path, "/wait") && e.Status == 200 {
				return true
			}
		}
		return false
	}) {
		return
	}
	l.heartbeat = w.lifecycleHeartbeat()
	l.signalled = time.Now() // transport-failure origin, not a synthetic signal
	l.fault.disconnect()
	w.offline.api.closeConnections()
}

func (w *world) lifecycleCrash() {
	l := w.lifecycle
	w.lifecycleStart()
	l.barrierReached = recoveryBarrier(l.fault.ready, l.run)
	if !l.barrierReached {
		w.lifecycleAbortEarly()
		return
	}
	w.recoveryDiscover()
	if l.barrier == "after accepted start before reply" {
		l.initialBoundary, l.err = w.offline.api.inspect(w.recovery.worker)
	}
	l.err = errors.Join(l.err, l.run.killController())
	w.lifecycleCollect()
	if l.barrier == "after accepted start before reply" {
		l.survivedLoss = w.recoveryNativePresent(true)
		var err error
		l.lostBoundary, err = w.offline.api.inspect(w.recovery.worker)
		l.err = errors.Join(l.err, err)
	}
	l.fault.restore()
	w.lifecycleFreshRecovery(true)
}

func (w *world) lifecycleStreams() {
	l := w.lifecycle
	if l.condition != "quiet workload and stdin" {
		if err := w.lifecycleControl(true); err != nil {
			l.err = err
			return
		}
	}
	w.lifecycleStart()
	if !w.lifecycleReady() {
		w.lifecycleAbortEarly()
		return
	}
	if l.condition == "quiet workload and stdin" {
		if w.duringQuiet != nil {
			w.duringQuiet()
		}
		if !lifecycleUntil(l.run, l.ready.Add(engineQuietInterval)) || !w.recoveryNativePresent(true) {
			w.lifecycleAbortEarly()
			return
		}
	}
	if l.pipe != nil {
		stdout := l.condition == "unread workload stdout"
		// Stdout's exact readiness prelude was already consumed by lifecycleReady.
		// Stderr still preserves only its pre-release diagnostics here.
		if !stdout {
			data, err := lifecyclePipePrelude(l.pipe, false)
			_, _ = l.run.errout.Write(data)
			if err != nil {
				l.err = errors.Join(l.err, err)
				w.lifecycleAbortEarly()
				return
			}
		}
		if err := os.WriteFile(filepath.Join(w.doctor.checkout, "slice-12-release-output"), []byte("release"), 0600); err != nil {
			l.err = err
			w.lifecycleAbortEarly()
			return
		}
		l.blocked = w.lifecycleAwait(5*time.Second, func() bool {
			return lifecycleFullPipe(l.pipe)
		})
		if !l.blocked {
			w.lifecycleAbortEarly()
			return
		}
	}
	w.lifecycleSignal("SIGTERM")
	w.lifecycleCollect()
	w.lifecycleFreshRecovery(false)
	if l.condition == "quiet workload and stdin" {
		observed := &lifecycleQuietEvidence{elapsed: l.signalled.Sub(l.ready), err: errors.Join(w.lifecycleReceipts("SIGTERM", 17), w.lifecycleComplete())}
		lifecycleQuietControls.Lock()
		lifecycleQuietControls.results[w.bin] = observed
		lifecycleQuietControls.Unlock()
	}
}

type lifecycleQuietEvidence struct {
	elapsed time.Duration
	err     error
}

var lifecycleQuietControls = struct {
	sync.Mutex
	results map[string]*lifecycleQuietEvidence
}{results: map[string]*lifecycleQuietEvidence{}}

// A healthy control executes the same command/transport/recorder path in an
// independent disposable checkout. It is an action, never a successful Given.
// The spec permits sharing one budget-relative quiet control within the stream outline.
// Cache only observed duration and a checked native result, scoped to the built
// binary. Every row still executes its own product invocation and coverage.
func (w *world) lifecycleControl(quiet bool) error {
	if quiet {
		lifecycleQuietControls.Lock()
		observed := lifecycleQuietControls.results[w.bin]
		lifecycleQuietControls.Unlock()
		if observed != nil {
			w.lifecycle.quiet = observed
			return observed.err
		}
	}
	control := &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(w.dir, "slice-12-control")}
	if err := os.Mkdir(control.dir, 0700); err != nil {
		return err
	}
	w.lifecycle.control = control
	if err := control.lifecycleFixture("handle", "quiet workload and stdin", "", 17, 0); err != nil {
		return err
	}
	if quiet {
		control.lifecycleStreams()
		lifecycleQuietControls.Lock()
		w.lifecycle.quiet = lifecycleQuietControls.results[w.bin]
		lifecycleQuietControls.Unlock()
	} else {
		control.lifecycleSignalAction("SIGTERM", false)
	}
	// Existing exit-17 behaviour is the healthy control. New typed cancellation
	// diagnostics are asserted separately on the product under test.
	return errors.Join(control.lifecycleReceipts("SIGTERM", 17), control.lifecycleComplete())
}
