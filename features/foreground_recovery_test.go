package features

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

type foregroundRecovery struct {
	fault         string
	block         bool
	trailing      bool
	lateStdin     bool
	waits         int
	discarded     int64
	exitCode      int
	started, exit chan struct{}
	finish        sync.Once
	began         sync.Once
	signals       []string
	elapsed       time.Duration
	startup       chan time.Time
	draining      chan struct{}
	signaled      chan struct{}
	done          chan struct{}
	notified      sync.Once
	expired       bool
	startupBudget time.Duration
	graceBudget   time.Duration
}

func (w *world) foregroundRecoverySteps(sc *godog.ScenarioContext) {
	w.foregroundInterruptSteps(sc)
	sc.Step(`^a foreground recovery fixture for "([^"]*)"$`, w.foregroundRecoveryFixture)
	sc.Step(`^a foreground recovery fixture with "([^"]*)" and trailing output after exit (\d+)$`, func(signal string, exit int) {
		w.foregroundRecoveryFixture("handled " + signal)
		w.doctor.engine.offline.mu.Lock()
		defer w.doctor.engine.offline.mu.Unlock()
		w.doctor.engine.offline.recovery.trailing = true
		w.doctor.engine.offline.recovery.exitCode = exit
	})
	sc.Step(`^a foreground recovery fixture with exit (\d+) and unread redirected stdin$`, func(exit int) {
		w.foregroundRecoveryFixture("unread stdin")
		w.doctor.engine.offline.mu.Lock()
		defer w.doctor.engine.offline.mu.Unlock()
		w.doctor.engine.offline.recovery.exitCode = exit
	})
	sc.Step(`^a foreground recovery fixture with exit (\d+) and unread stdin after the startup deadline$`, func(exit int) {
		w.foregroundRecoveryFixture("unread stdin")
		w.doctor.engine.offline.mu.Lock()
		defer w.doctor.engine.offline.mu.Unlock()
		f := w.doctor.engine.offline.recovery
		f.exitCode, f.lateStdin, f.trailing = exit, true, true
	})
	sc.Step(`^the foreground recovery command runs$`, w.foregroundRecoveryRun)
	sc.Step(`^the foreground recovery command preserves both trailing streams and the interrupted exit$`, func() error {
		f := w.doctor.engine.offline.recovery
		want := f.exitCode
		if w.exit != want || !strings.Contains(w.stderr, "run.cancelled") || w.stdout != recoveryTailOut || !strings.HasPrefix(w.stderr, recoveryTailErr) {
			return fmt.Errorf("interrupted exit lost trailing stdout/stderr or worker status: %s", w.report())
		}
		return nil
	})
	sc.Step(`^the foreground recovery command preserves the worker exit despite unread stdin$`, func() error {
		f := w.doctor.engine.offline.recovery
		if w.exit != f.exitCode || w.stdout != "" || w.stderr != "" || f.waits != 2 || f.discarded >= 4<<20 {
			return fmt.Errorf("unread stdin replaced the worker exit with a stream failure: %s", w.report())
		}
		return nil
	})
	sc.Step(`^the foreground recovery command preserves the late worker exit and trailing streams$`, func() error {
		f := w.doctor.engine.offline.recovery
		if w.exit != f.exitCode || w.stdout != recoveryTailOut || w.stderr != recoveryTailErr ||
			!f.expired || f.startupBudget < 119*time.Second || f.startupBudget > 120*time.Second || f.elapsed >= 5*time.Second || f.waits != 2 || f.discarded >= 4<<20 {
			return fmt.Errorf("startup deadline lost the late worker exit or trailing streams: elapsed=%v waits=%d %s", f.elapsed, f.waits, w.report())
		}
		return nil
	})
	sc.Step(`^the foreground recovery command has confirmed owned cleanup$`, w.localOfflineOwned)
	sc.Step(`^the foreground recovery command reports stream failure and completes owned cleanup$`, func() error {
		if w.exit != 75 || (w.doctor.engine.offline.recovery.fault != "stderr" && !strings.Contains(w.stderr, "run.execution-unavailable")) {
			return fmt.Errorf("stream failure did not return execution failure: %s", w.report())
		}
		return w.localOfflineOwned()
	})
	sc.Step(`^the foreground recovery command succeeds with confirmed owned cleanup$`, func() error {
		if w.exit != 0 {
			return fmt.Errorf("checkout change vetoed owned cleanup: %s", w.report())
		}
		return w.localOfflineOwned()
	})
	sc.Step(`^the foreground recovery command forwards once and completes bounded owned cleanup$`, func() error {
		a := w.doctor.engine.offline
		a.mu.Lock()
		f := a.recovery
		_, signal, _ := strings.Cut(f.fault, " ")
		want := []string{signal}
		if strings.HasPrefix(f.fault, "ignored") {
			want = append(want, "SIGKILL")
		}
		valid := reflect.DeepEqual(f.signals, want) && f.elapsed < 5*time.Second
		if strings.HasPrefix(f.fault, "ignored") {
			valid = valid && f.elapsed >= 100*time.Millisecond && f.graceBudget > 9*time.Second && f.graceBudget <= 10*time.Second
		}
		a.mu.Unlock()
		if !valid || w.exit != 0 || !strings.Contains(w.stderr, "run.cancelled") || !strings.Contains(w.stderr, signal) {
			return fmt.Errorf("interrupt did not forward/escalate and return failure: signals=%v elapsed=%v %s", f.signals, f.elapsed, w.report())
		}
		return w.localOfflineOwned()
	})
}

const recoveryTailOut = "final stdout\x00\xff\n"
const recoveryTailErr = "final stderr\x00\xfe\n"

func (w *world) foregroundRecoveryFixture(fault string) {
	w.localOfflineFixture()
	w.doctor.engine.offline.mu.Lock()
	defer w.doctor.engine.offline.mu.Unlock()
	w.doctor.engine.offline.recovery = &foregroundRecovery{fault: fault, block: fault == "stdout" || fault == "stderr" || strings.Contains(fault, "SIG"),
		started: make(chan struct{}), exit: make(chan struct{}), startup: make(chan time.Time, 1), draining: make(chan struct{}), signaled: make(chan struct{}), done: make(chan struct{})}
}

func (a *localOfflineAPI) recoveryStart(out http.ResponseWriter, resource map[string]any, id string) {
	f := a.recovery
	resource["State"] = map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-01-01T00:00:00Z"}
	conn := a.attached[id]
	switch f.fault {
	case "stdout", "stderr":
		stream := byte(1)
		if f.fault == "stderr" {
			stream = 2
		}
		frame := make([]byte, 8)
		frame[0] = stream
		binary.BigEndian.PutUint32(frame[4:], 5)
		_, a.problem = conn.Write(append(frame, []byte("probe")...))
	case "stdin":
		// The read must finish before reporting exit, as a cat-like worker does.
		_, a.problem = io.Copy(io.Discard, conn)
		_ = conn.Close()
	case "unread stdin":
		// Reject input once the first wait is acknowledged. This forces the real
		// input-closure notification to cancel that wait before exit reconciliation.
	case "replaced git", "missing git":
		git := filepath.Join(a.w.doctor.checkout, ".git")
		a.problem = os.Rename(git, git+".before-run")
		if a.problem == nil && f.fault == "replaced git" {
			a.problem = os.Mkdir(git, 0700)
		}
		_ = conn.Close()
	}
	out.WriteHeader(204)
}

func (w *world) foregroundRecoveryRun() error {
	a := w.doctor.engine.offline
	f := a.recovery
	assembly := f.lateStdin || f.trailing || strings.HasPrefix(f.fault, "ignored")
	bin, args := w.bin, []string{"run", "--", "probe"}
	if assembly {
		var err error
		bin, err = w.sessionProbeBinary()
		if err != nil {
			return err
		}
		args = []string{"-test.run=^TestForegroundRecoveryCommand$", "-test.count=1"}
		if cover := w.coverageDir(); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
	}
	if !assembly {
		var err error
		bin, err = w.sessionProbeBinary()
		if err != nil {
			return err
		}
		args = append([]string{"-test.run=^TestLocalRunCommand$", "-test.count=1", "--"}, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir, cmd.Env = w.doctor.cwd, append(w.env(), "SLICE_17_LOCAL_RUN=1", "SLICE_17_LOCAL_MOUNT_ROOT="+w.doctor.checkout)
	result := filepath.Join(w.dir, "slice-10-recovery-result.json")
	stdout, stderr := filepath.Join(w.dir, "slice-10-recovery.out"), filepath.Join(w.dir, "slice-10-recovery.err")
	if f.fault == "stdout" {
		stdout = "/dev/full"
	}
	if f.fault == "stderr" {
		stderr = "/dev/full"
	}
	cmd.Env = append(cmd.Env, "SLICE_10_RECOVERY_RESULT="+result, "SLICE_10_RECOVERY_STDOUT="+stdout, "SLICE_10_RECOVERY_STDERR="+stderr)
	if f.lateStdin {
		cmd.Env = append(cmd.Env, "SLICE_10_RECOVERY_LATE=1")
	}
	if strings.HasPrefix(f.fault, "ignored") {
		cmd.Env = append(cmd.Env, "SLICE_10_RECOVERY_IGNORED=1")
	}
	read, write, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = read.Close(); _ = write.Close() }()
	cmd.ExtraFiles = []*os.File{write}
	var transcript strings.Builder
	cmd.Stdout, cmd.Stderr = &transcript, &transcript
	if !assembly {
		out, err := os.OpenFile(stdout, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer func() { _ = out.Close() }()
		errout, err := os.OpenFile(stderr, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer func() { _ = errout.Close() }()
		cmd.Stdout, cmd.Stderr = out, errout
	}
	cmd.WaitDelay = time.Second
	if f.fault == "unread stdin" {
		file, err := os.CreateTemp(w.doctor.checkout, "unread-input-*")
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		// Larger than the socket buffers, so forwarding cannot finish before exit.
		if err := file.Truncate(4 << 20); err != nil {
			return err
		}
		cmd.Stdin = file
	}
	if f.fault == "stdin" {
		file, err := os.Open(w.doctor.checkout)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		cmd.Stdin = file
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = write.Close()
	observed := make(chan struct{})
	go func() {
		defer close(observed)
		decoder := json.NewDecoder(read)
		for {
			var event struct {
				Name     string
				Deadline time.Time
				Budget   time.Duration
			}
			if decoder.Decode(&event) != nil {
				return
			}
			a.mu.Lock()
			switch event.Name {
			case "startup":
				f.startupBudget = event.Budget
				f.startup <- event.Deadline
			case "grace":
				f.graceBudget = event.Budget
			case "drain":
				close(f.draining)
			}
			a.mu.Unlock()
		}
	}()
	a.mu.Lock()
	a.expected[cmd.Process.Pid] = []string{"probe"}
	a.mu.Unlock()
	done := f.done
	go func() { _ = cmd.Wait(); close(done) }()
	if strings.Contains(f.fault, "SIG") {
		select {
		case <-f.started:
			signal := syscall.SIGINT
			if strings.HasSuffix(f.fault, "SIGTERM") {
				signal = syscall.SIGTERM
			}
			_ = cmd.Process.Signal(signal)
			if strings.HasPrefix(f.fault, "ignored") {
				select {
				case <-f.signaled:
					_ = cmd.Process.Signal(signal)
				case <-done:
				}
			}
		case <-done:
		case <-ctx.Done():
		}
	}
	<-done
	<-observed
	f.elapsed = time.Since(start)
	w.exit = cmd.ProcessState.ExitCode()
	if assembly {
		data, err := os.ReadFile(result)
		if err != nil {
			w.stderr = transcript.String()
			return nil
		}
		if err := json.Unmarshal(data, &w.exit); err != nil {
			return err
		}
	}
	if f.fault != "stdout" {
		data, err := os.ReadFile(stdout)
		if err != nil {
			return err
		}
		w.stdout = string(data)
	}
	if f.fault != "stderr" {
		data, err := os.ReadFile(stderr)
		if err != nil {
			return err
		}
		w.stderr = string(data)
	}
	return nil // Missing launch, signal death and deadline are assertion observations.
}
