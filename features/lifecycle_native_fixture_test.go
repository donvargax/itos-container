package features

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type lifecycleNative struct {
	mode, condition, barrier, first string
	child                           int
	delay                           time.Duration
	fault                           *lifecycleFaults
	run                             *recoveryProcess
	stdin, pipe                     *os.File
	signalTrace                     string
	ready, signalled, returned      time.Time
	exitObserved                    time.Time
	aliveBeforeGrace, blocked       bool
	barrierReached                  bool
	survivedLoss                    bool
	heartbeat, later                int
	initialBoundary, lostBoundary   map[string]any
	firstRecord, firstIntent        sessionRecord
	journalAtReturn                 []offlineRequest
	control                         *world
	quiet                           *lifecycleQuietEvidence
	err                             error
}

func (w *world) lifecycleFixture(mode, condition, barrier string, child, delay int) error {
	if err := w.recoveryNativeFixture("lifecycle"); err != nil {
		return err
	}
	l := &lifecycleNative{mode: mode, condition: condition, barrier: barrier, child: child, delay: time.Duration(delay) * time.Second}
	w.lifecycle = l
	l.fault = newLifecycleFaults(mode, barrier, l.delay)
	w.offline.api.lifecycle = l.fault
	if err := w.buildLifecycleProbe(); err != nil {
		return err
	}
	var err error
	w.recovery.before, err = w.lifecycleProtected()
	return err
}

func (w *world) lifecycleProtected() (map[string]string, error) {
	all, err := w.recoveryProtected()
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"worker-edit", "ready", "temporary", "heartbeat", "receipts", "release-output", "fill-started", "fill-finished"} {
		delete(all, filepath.Join(w.doctor.checkout, "slice-12-"+name))
		delete(all, filepath.Join(w.doctor.checkout, "slice-12-"+name+".next"))
	}
	return all, nil
}

func (w *world) lifecycleStart() {
	l, r := w.lifecycle, w.recovery
	p := &recoveryProcess{exit: -1, done: make(chan struct{})}
	l.run, r.run = p, p
	r.processes = append(r.processes, p)
	p.journal, p.pid = filepath.Join(w.dir, "slice-12-sync.jsonl"), filepath.Join(w.dir, "slice-12-controller.pid")
	l.signalTrace = filepath.Join(w.dir, "slice-12-signals.log")
	trace, err := w.recoveryTraceBinary()
	if err != nil {
		p.err = err
		close(p.done)
		return
	}
	// Long enough for the specified 125 s quiet observation and separate 30 s
	// teardown. This is a harness guard, never a passing product deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 190*time.Second)
	p.cancel = cancel
	probeMode := "handle"
	if l.mode == "ignore" {
		probeMode = "ignore"
	}
	if l.mode == "cleanup-signals" || (l.mode == "crash" && strings.Contains(l.barrier, "removal")) {
		probeMode = "exit"
	}
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	p.cmd = exec.CommandContext(ctx, trace, p.journal, p.pid, state, w.bin, "run", "--", "/workspace/slice-12-probe", probeMode, strconv.Itoa(l.child), l.condition)
	p.cmd.Dir, p.cmd.Env = w.doctor.checkout, w.env()
	for key, value := range w.doctor.env {
		p.cmd.Env = setEnv(p.cmd.Env, key, value)
	}
	p.cmd.Env = setEnv(p.cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	p.cmd.Env = setEnv(p.cmd.Env, "SLICE_11_SIGNAL_TRACE", l.signalTrace)
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.errout
	input, writer, err := os.Pipe()
	if err != nil {
		p.err = err
		cancel()
		close(p.done)
		return
	}
	l.stdin, p.cmd.Stdin = writer, input
	defer func() { _ = input.Close() }()
	var output *os.File
	if strings.HasPrefix(l.condition, "unread workload") {
		l.pipe, output, err = os.Pipe()
		if err != nil {
			p.err = err
			cancel()
			close(p.done)
			return
		}
		defer func() { _ = output.Close() }()
		if l.condition == "unread workload stdout" {
			p.cmd.Stdout = output
		} else {
			p.cmd.Stderr = output
		}
	}
	p.started = time.Now()
	if err := p.cmd.Start(); err != nil {
		p.err = err
		cancel()
		close(p.done)
		return
	}
	go func() {
		defer close(p.done)
		defer cancel()
		err := p.cmd.Wait()
		p.finished = time.Now()
		p.exit = 0
		if err != nil {
			var exited *exec.ExitError
			if errors.As(err, &exited) {
				p.exit = exited.ExitCode()
			} else {
				p.err = err
			}
		}
		if ctx.Err() != nil {
			p.err = errors.New("lifecycle controller exceeded the harness guard")
		}
	}()
}

// A missing barrier or an early product exit is data for Then, never a failing
// Given or a substitute cancellation/cleanup success supplied by the harness.
func (w *world) lifecycleAwait(timeout time.Duration, condition func() bool) bool {
	p := w.lifecycle.run
	timer, tick := time.NewTimer(timeout), time.NewTicker(10*time.Millisecond)
	defer timer.Stop()
	defer tick.Stop()
	for {
		select {
		case <-p.done:
			return false
		case <-timer.C:
			return false
		case <-tick.C:
			if condition() {
				return true
			}
		}
	}
}

func (w *world) lifecycleReady() bool {
	l := w.lifecycle
	if !w.lifecycleAwait(35*time.Second, func() bool {
		data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-12-ready"))
		return err == nil && string(data) == "ready" && w.offline.api.running()
	}) {
		return false
	}
	w.recoveryDiscover()
	l.ready = time.Now()
	l.initialBoundary, l.err = w.offline.api.inspect(w.recovery.worker)
	return l.err == nil
}

func (w *world) lifecycleSignal(signal string) {
	l := w.lifecycle
	select {
	case <-l.run.done:
		l.err = errors.Join(l.err, errors.New("controller exited before requested signal"))
		return
	default:
	}
	data, err := os.ReadFile(l.run.pid)
	pid, parseErr := strconv.Atoi(string(data))
	if err != nil || parseErr != nil || pid <= 0 {
		l.err = errors.Join(l.err, errors.New("observer did not identify its controller"))
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		l.err = errors.Join(l.err, err)
		return
	}
	defer func() { _ = process.Release() }()
	number := syscall.Signal(15)
	if signal == "SIGINT" {
		number = syscall.Signal(2)
	}
	if l.first == "" {
		l.first, l.signalled = signal, time.Now()
	}
	l.err = errors.Join(l.err, process.Signal(number))
}

func (w *world) lifecycleCollect() {
	l := w.lifecycle
	l.run.wait()
	l.returned = l.run.finished
	l.err = errors.Join(l.err, l.run.err)
	if l.stdin != nil {
		_ = l.stdin.Close()
	}
	// Closing or reading a blocked pipe is deliberately AFTER independent
	// process termination. It cannot help the controller satisfy its deadline.
	if l.pipe != nil {
		_ = l.pipe.SetReadDeadline(time.Now().Add(2 * time.Second))
		data, err := io.ReadAll(l.pipe)
		_ = l.pipe.Close()
		l.err = errors.Join(l.err, err)
		if l.condition == "unread workload stdout" {
			_, _ = l.run.out.Write(data)
		} else {
			_, _ = l.run.errout.Write(data)
		}
	}
	w.recoveryDiscover()
	a := w.offline.api
	a.mu.Lock()
	l.journalAtReturn = append([]offlineRequest(nil), a.requests...)
	l.err = errors.Join(l.err, a.problem)
	a.mu.Unlock()
	if offlineHex(w.recovery.id) {
		path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey(), w.recovery.id, "record.json")
		var err error
		l.firstRecord, err = sessionReadRecord(path)
		l.err = errors.Join(l.err, err)
		l.firstIntent, err = sessionReadRecord(w.sessionRecordPath())
		l.err = errors.Join(l.err, err)
	}
}

func (w *world) closeLifecycleNative() error {
	l := w.lifecycle
	if l == nil {
		return nil
	}
	err := w.closeRecoveryNative()
	l.fault.restore()
	if l.stdin != nil {
		_ = l.stdin.Close()
	}
	if l.pipe != nil {
		_ = l.pipe.Close()
	}
	if l.control != nil {
		c := l.control
		err = errors.Join(err, c.closeLifecycleNative(), c.closeRecoveryNative(), c.closeOfflineRuntime(), c.closeSession())
		c.closeDoctor()
	}
	return err
}

func (w *world) lifecycleFreshRecovery(stop bool) {
	r := w.recovery
	r.status = w.recoveryStart("status", "--json")
	r.status.wait()
	// Use only the public status reference, never the observer's resource ID
	// or the intent name, as authority for the fresh product invocation.
	report, err := recoveryReadReport(r.status)
	if err == nil && len(report.Sessions) == 1 && offlineHex(report.Sessions[0].ID) && stop {
		entry := report.Sessions[0]
		if len(entry.Retry) == 4 && entry.Retry[0] == "itos" && entry.Retry[1] == "container" && entry.Retry[2] == "stop" && entry.Retry[3] == entry.ID {
			p := w.recoveryStart("stop", "--json", entry.ID)
			p.wait()
			r.stops = append(r.stops, p)
		}
	}
	r.after = w.recoveryStart("status", "--json")
	r.after.wait()
}

func (w *world) lifecycleAbortEarly() {
	l := w.lifecycle
	l.err = errors.Join(l.err, fmt.Errorf("product did not reach the %s lifecycle barrier", l.mode))
	if l.run.cancel != nil {
		l.run.cancel()
	}
	w.lifecycleCollect()
}
