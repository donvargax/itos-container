package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type recoveryNative struct {
	mode, id, worker string
	second           *world
	processes        []*recoveryProcess
	run              *recoveryProcess
	stops            []*recoveryProcess
	status, after    *recoveryProcess
	err              error
	before           map[string]string
	snapshot         sessionRecord
	heartbeat        int
	laterHeartbeat   int
	retained         bool
	retry            []string
}

type recoveryProcess struct {
	cmd               *exec.Cmd
	cancel            context.CancelFunc
	done              chan struct{}
	out, errout       offlineBuffer
	exit              int
	err               error
	journal, pid      string
	started, finished time.Time
}

func (w *world) recoveryNativeFixture(mode string) error {
	return w.recoveryFixtureWithProbe(mode, w.buildRecoveryProbe)
}

func (w *world) buildRecoveryProbe() error {
	return w.installNativeProbe("slice-11-probe", func(path string) error {
		return recoveryBuildHelper(w.root, path, recoveryWorkerSource)
	})
}

func (w *world) recoveryFixtureWithProbe(mode string, buildProbe func() error) error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	w.recovery = &recoveryNative{mode: mode}
	if err := w.offlineFixtureWithProbe("recovery", buildProbe); err != nil {
		return err
	}
	a := w.offline.api
	a.mu.Lock()
	a.recovery, a.failRemove = true, mode == "retry"
	if mode == "pair" {
		a.removeGate, a.removeReady = make(chan struct{}), make(chan struct{})
	}
	a.mu.Unlock()
	if _, err := w.recoveryTraceBinary(); err != nil {
		return err
	}
	var err error
	w.recovery.before, err = w.recoveryProtected()
	return err
}

func (w *world) recoveryPairFixture() error {
	if err := w.recoveryNativeFixture("pair"); err != nil {
		return err
	}
	child := &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(w.dir, "slice-11-project-b")}
	w.recovery.second = child
	if err := os.Mkdir(child.dir, 0700); err != nil {
		return err
	}
	if err := child.recoveryNativeFixture("pair"); err != nil {
		return err
	}
	if filepath.Base(w.doctor.checkout) != filepath.Base(child.doctor.checkout) || w.doctor.checkout == child.doctor.checkout {
		return fmt.Errorf("fixture did not produce distinct equal-basename checkouts")
	}
	return nil
}

func (w *world) recoveryProtected() (map[string]string, error) {
	all, err := w.offlineProtected()
	if err != nil {
		return nil, err
	}
	sessions := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions")
	tree, err := runTree(filepath.Dir(sessions))
	if err != nil {
		return nil, err
	}
	admission, err := w.currentAdmissionPaths(tree)
	if err != nil {
		return nil, err
	}
	for path := range admission {
		delete(all, path)
	}
	for path := range all {
		if path == sessions || path == filepath.Join(sessions, w.recoveryIndexKey()) ||
			strings.HasPrefix(path, filepath.Join(sessions, w.recoveryIndexKey())+string(os.PathSeparator)) ||
			path == filepath.Join(w.doctor.checkout, "slice-11-heartbeat") || path == filepath.Join(w.doctor.checkout, "slice-11-heartbeat.next") ||
			path == filepath.Join(w.doctor.checkout, "slice-11-worker-edit") {
			delete(all, path)
		}
	}
	return all, nil
}

func (w *world) recoveryStart(args ...string) *recoveryProcess {
	return w.recoveryStartWithin(120*time.Second, args...)
}

func (w *world) recoveryStartWithin(timeout time.Duration, args ...string) *recoveryProcess {
	return w.recoveryStartConfigured(timeout, w.bin, nil, args...)
}

func (w *world) recoveryStartConfigured(timeout time.Duration, executable string, env map[string]string, args ...string) *recoveryProcess {
	r := w.recovery
	p := &recoveryProcess{exit: -1, done: make(chan struct{})}
	if w.session != nil && w.session.api != nil {
		w.session.api.mu.Lock()
	}
	r.processes = append(r.processes, p)
	index := strconv.Itoa(len(r.processes))
	p.journal = filepath.Join(w.dir, "slice-11-sync-"+index+".jsonl")
	p.pid = filepath.Join(w.dir, "slice-11-pid-"+index)
	if w.session != nil && w.session.api != nil {
		w.session.api.mu.Unlock()
	}
	trace, err := w.recoveryTraceBinary()
	if err != nil {
		p.err = err
		close(p.done)
		return p
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	p.cancel = cancel
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	localRun := w.doctor.real == nil && len(args) > 0 && args[0] == "run"
	if localRun {
		executable, err = w.sessionProbeBinary()
		if err != nil {
			p.err = err
			cancel()
			close(p.done)
			return p
		}
		args = append([]string{"-test.run=^TestLocalRunCommand$", "-test.count=1", "--"}, args...)
	}
	p.cmd = exec.CommandContext(ctx, trace, append([]string{p.journal, p.pid, state, executable}, args...)...)
	p.cmd.WaitDelay = time.Second
	p.cmd.Dir, p.cmd.Env = w.doctor.checkout, w.env()
	if w.recoveryLocal != nil && w.recoveryLocal.invocationDir != "" {
		p.cmd.Dir = w.recoveryLocal.invocationDir
	}
	for key, value := range w.doctor.env {
		p.cmd.Env = setEnv(p.cmd.Env, key, value)
	}
	for key, value := range env {
		p.cmd.Env = setEnv(p.cmd.Env, key, value)
	}
	if w.anchors != nil {
		p.cmd.Env = setEnv(p.cmd.Env, "SLICE_17_ACCESS_TRACE", "1")
	}
	p.cmd.Env = setEnv(p.cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	if localRun {
		p.cmd.Env = setEnv(p.cmd.Env, "SLICE_17_LOCAL_RUN", "1")
		p.cmd.Env = setEnv(p.cmd.Env, "SLICE_17_LOCAL_MOUNT_ROOT", w.doctor.checkout)
	}
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.errout
	p.started = time.Now()
	if err := w.startLocalCommand(p.cmd); err != nil {
		p.err = err
		cancel()
		close(p.done)
		return p
	}
	go func() {
		defer close(p.done)
		defer cancel()
		err := p.cmd.Wait()
		unlock := w.lockLocalAPI()
		defer unlock()
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
			p.err = errors.New("recovery product exceeded the harness deadline")
		}
	}()
	return p
}

func (p *recoveryProcess) wait() { <-p.done }

func (p *recoveryProcess) killController() error {
	select {
	case <-p.done:
		return fmt.Errorf("controller exited before the loss barrier (exit %d)", p.exit)
	default:
	}
	data, err := os.ReadFile(p.pid)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		return errors.New("observer did not record its child PID")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer func() { _ = process.Release() }()
	return process.Kill()
}

func (w *world) recoveryAwaitWorker() {
	r := w.recovery
	deadline := time.NewTimer(40 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.run.done:
			// Refusal is product data for the positive Then, not a setup error.
			return
		case <-deadline.C:
			r.err = errors.New("native worker did not reach the bounded heartbeat barrier")
			return
		case <-ticker.C:
			if !w.offline.api.running() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-11-heartbeat"))
			value, parseErr := strconv.Atoi(string(data))
			if err == nil && parseErr == nil && value > 0 {
				r.heartbeat = value
				w.recoveryDiscover()
				return
			}
		}
	}
}

func (w *world) recoveryDiscover() {
	r, a := w.recovery, w.offline.api
	a.mu.Lock()
	if len(a.created) == 1 {
		r.worker = a.created[0]
	}
	a.mu.Unlock()
	path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey())
	entries, err := os.ReadDir(path)
	if err == nil && len(entries) == 1 && entries[0].IsDir() && offlineHex(entries[0].Name()) {
		r.id = entries[0].Name()
	}
	snapshot := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", w.approval.project, "1.json")
	r.snapshot, err = sessionReadRecord(snapshot)
	if err != nil {
		r.err = errors.Join(r.err, err)
	}
}

var recoveryRetryPattern = regexp.MustCompile(`(?:^|\n)[^\n]*\bitos container stop ([0-9a-f]{64})(?:\r?\n|$)`)

func (w *world) recoveryRetryAction() {
	r := w.recovery
	r.run = w.recoveryStart("run", "--", "/workspace/slice-11-probe", "exit")
	r.run.wait()
	w.recoveryDiscover()
	r.retained = w.recoveryNativePresent(false)
	matches := recoveryRetryPattern.FindAllSubmatch(r.run.errout.bytes(), -1)
	if len(matches) == 1 {
		r.retry = []string{"stop", string(matches[0][1])}
	}
	a := w.offline.api
	a.mu.Lock()
	a.failRemove = false
	a.mu.Unlock()
	if len(r.retry) != 0 {
		stop := w.recoveryStart(r.retry...)
		stop.wait()
		r.stops = append(r.stops, stop)
	}
	r.after = w.recoveryStart("status", "--json")
	r.after.wait()
}

func (w *world) recoveryLossAction() {
	r := w.recovery
	r.run = w.recoveryStart("run", "--", "/workspace/slice-11-probe", "heartbeat")
	w.recoveryAwaitWorker()
	if r.worker != "" {
		r.err = errors.Join(r.err, r.run.killController())
		r.run.wait()
		w.recoveryLaterHeartbeat()
	}
	r.status = w.recoveryStart("status", "--json")
	r.status.wait()
	if offlineHex(r.id) {
		stop := w.recoveryStart("stop", r.id, "--json")
		stop.wait()
		r.stops = append(r.stops, stop)
	}
	r.after = w.recoveryStart("status", "--json")
	r.after.wait()
}

func (w *world) recoveryPairAction() {
	other := w.recovery.second
	for _, project := range []*world{w, other} {
		project.recovery.run = project.recoveryStart("run", "--", "/workspace/slice-11-probe", "heartbeat")
		project.recoveryAwaitWorker()
	}
	r := w.recovery
	if offlineHex(r.id) {
		// Park A's first removal before starting the second process, so a fast
		// first stop cannot turn the concurrency case into two sequential stops.
		first := w.recoveryStart("stop", r.id, "--json")
		timer := time.NewTimer(32 * time.Second)
		select {
		case <-w.offline.api.removeReady:
		case <-first.done:
		case <-timer.C:
		}
		timer.Stop()
		second := w.recoveryStart("stop", r.id, "--json")
		r.stops = append(r.stops, first, second)
		close(w.offline.api.removeGate)
		first.wait()
		second.wait()
	}
	if len(r.stops) == 2 && r.stops[0].exit == 0 && r.stops[1].exit == 0 {
		r.run.wait()
	}
	other.recoveryLaterHeartbeat()
	for _, project := range []*world{w, other} {
		project.recovery.after = project.recoveryStart("status", "--json")
		project.recovery.after.wait()
	}
}

func (w *world) recoveryLaterHeartbeat() {
	r := w.recovery
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			return
		case <-ticker.C:
			data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-11-heartbeat"))
			value, parseErr := strconv.Atoi(string(data))
			if err == nil && parseErr == nil && value > r.heartbeat && w.recoveryNativePresent(true) {
				r.laterHeartbeat = value
				return
			}
		}
	}
}

func (w *world) recoveryNativePresent(running bool) bool {
	id := w.recovery.worker
	if !offlineHex(id) {
		return false
	}
	a := w.offline.api
	// The independent read must not hold the recorder lock across I/O: delayed
	// observations must not delay product signals, escalation or journaling.
	observed, err := a.inspect(id)
	a.mu.Lock()
	defer a.mu.Unlock()
	return err == nil && a.identity(observed, id) == nil && object(observed["State"])["Running"] == running
}

func (w *world) closeRecoveryNative() error {
	if w.recovery == nil {
		return nil
	}
	r := w.recovery
	for _, p := range r.processes {
		if p.cancel != nil {
			p.cancel() // tracer death kills only its controller, not another PID
		}
		p.wait()
	}
	anchorErr := w.closeAnchorsForeign()
	if r.second != nil {
		other := r.second
		err := errors.Join(anchorErr, other.closeRecoveryNative(), other.closeOfflineRuntime(), other.closeAnchor(), other.closeSession())
		other.closeDoctor()
		return err
	}
	return anchorErr
}

// Only a product worker executes this trusted static probe. Heartbeats are
// atomically replaced so host observations cannot mistake a partial write for
// progress. Their monotonic counter proves activity after controller loss/stop.
const recoveryWorkerSource = `package main
import ("os"; "strconv"; "time")
func main() {
    if len(os.Args) != 2 { os.Exit(96) }
    if os.WriteFile("/workspace/slice-11-worker-edit", []byte("slice-11 retained edit"), 0600) != nil { os.Exit(97) }
    if os.Args[1] == "exit" { os.Stdout.WriteString("slice-11 deliberate workload marker\n"); os.Exit(7) }
    if os.Args[1] != "heartbeat" { os.Exit(96) }
    for n := 1; ; n++ {
        if os.WriteFile("/workspace/slice-11-heartbeat.next", []byte(strconv.Itoa(n)), 0600) != nil { os.Exit(97) }
        if os.Rename("/workspace/slice-11-heartbeat.next", "/workspace/slice-11-heartbeat") != nil { os.Exit(97) }
        time.Sleep(50*time.Millisecond)
    }
}
`

func recoveryJSONEqual(data string, want map[string]any) bool {
	var got map[string]any
	return json.Unmarshal([]byte(data), &got) == nil && string(approvalJSON(got)) == string(approvalJSON(want))
}
