package features

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
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
	exitCode      int
	started, exit chan struct{}
	finish        sync.Once
	signals       []string
	elapsed       time.Duration
}

func (w *world) foregroundRecoverySteps(sc *godog.ScenarioContext) {
	sc.Step(`^a foreground recovery fixture for "([^"]*)"$`, w.foregroundRecoveryFixture)
	sc.Step(`^a foreground recovery fixture with "([^"]*)" and trailing output after exit (\d+)$`, func(signal string, exit int) {
		w.foregroundRecoveryFixture("handled " + signal)
		w.doctor.engine.offline.recovery.trailing = true
		w.doctor.engine.offline.recovery.exitCode = exit
	})
	sc.Step(`^a foreground recovery fixture with exit (\d+) and unread redirected stdin$`, func(exit int) {
		w.foregroundRecoveryFixture("unread stdin")
		w.doctor.engine.offline.recovery.exitCode = exit
	})
	sc.Step(`^a foreground recovery fixture with exit (\d+) and unread stdin after the startup deadline$`, func(exit int) {
		w.foregroundRecoveryFixture("unread stdin")
		f := w.doctor.engine.offline.recovery
		f.exitCode, f.lateStdin, f.trailing = exit, true, true
	})
	sc.Step(`^the foreground recovery command runs$`, w.foregroundRecoveryRun)
	sc.Step(`^the foreground recovery command preserves both trailing streams and the interrupted exit$`, func() error {
		f := w.doctor.engine.offline.recovery
		want := f.exitCode
		if want == 0 {
			want = 75
		}
		if w.exit != want || w.stdout != recoveryTailOut || !strings.HasPrefix(w.stderr, recoveryTailErr) {
			return fmt.Errorf("interrupted exit lost trailing stdout/stderr or worker status: %s", w.report())
		}
		return nil
	})
	sc.Step(`^the foreground recovery command preserves the worker exit despite unread stdin$`, func() error {
		if w.exit != w.doctor.engine.offline.recovery.exitCode || w.stdout != "" || w.stderr != "" {
			return fmt.Errorf("unread stdin replaced the worker exit with a stream failure: %s", w.report())
		}
		return nil
	})
	sc.Step(`^the foreground recovery command preserves the late worker exit and trailing streams$`, func() error {
		f := w.doctor.engine.offline.recovery
		if w.exit != f.exitCode || w.stdout != recoveryTailOut || w.stderr != recoveryTailErr ||
			f.elapsed < 120*time.Second || f.elapsed >= 135*time.Second {
			return fmt.Errorf("startup deadline lost the late worker exit or trailing streams: elapsed=%v %s", f.elapsed, w.report())
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
		valid := reflect.DeepEqual(f.signals, want) && f.elapsed < 25*time.Second
		if strings.HasPrefix(f.fault, "ignored") {
			valid = valid && f.elapsed >= 10*time.Second
		}
		a.mu.Unlock()
		if !valid || w.exit != 75 {
			return fmt.Errorf("interrupt did not forward/escalate and return failure: signals=%v elapsed=%v %s", f.signals, f.elapsed, w.report())
		}
		return w.localOfflineOwned()
	})
}

const recoveryTailOut = "final stdout\x00\xff\n"
const recoveryTailErr = "final stderr\x00\xfe\n"

func (w *world) foregroundRecoveryFixture(fault string) {
	w.localOfflineFixture()
	w.doctor.engine.offline.recovery = &foregroundRecovery{fault: fault, block: fault == "stdout" || fault == "stderr" || strings.Contains(fault, "SIG"), started: make(chan struct{}), exit: make(chan struct{})}
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
		// The worker rejects further input but finishes output cleanly. Closing
		// with unread socket data instead would also reset stdout/stderr, which
		// is a separate transport failure, not this stdin-only regression.
		if !f.lateStdin {
			_ = conn.(*net.UnixConn).CloseRead()
			_ = conn.(*net.UnixConn).CloseWrite()
		}
	case "replaced git", "missing git":
		git := filepath.Join(a.w.doctor.checkout, ".git")
		a.problem = os.Rename(git, git+".before-run")
		if a.problem == nil && f.fault == "replaced git" {
			a.problem = os.Mkdir(git, 0700)
		}
		_ = conn.Close()
	}
	if !f.block && !f.lateStdin {
		resource["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
	}
	close(f.started)
	out.WriteHeader(204)
}

func (w *world) foregroundRecoveryRun() error {
	a := w.doctor.engine.offline
	f := a.recovery
	budget := 5 * time.Second
	if strings.Contains(f.fault, "SIG") {
		budget = 25 * time.Second
	}
	if f.lateStdin {
		budget = 140 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, "run", "--", "probe")
	cmd.Dir, cmd.Env = w.doctor.cwd, w.env()
	var out, errout strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errout
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
	if f.fault == "stdin" || f.fault == "stdout" || f.fault == "stderr" {
		path := "/dev/full"
		mode := os.O_WRONLY
		if f.fault == "stdin" {
			path, mode = w.doctor.checkout, os.O_RDONLY
		}
		file, err := os.OpenFile(path, mode, 0)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		switch f.fault {
		case "stdin":
			cmd.Stdin = file
		case "stdout":
			cmd.Stdout = file
		case "stderr":
			cmd.Stderr = file
		}
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	a.mu.Lock()
	a.expected[cmd.Process.Pid] = []string{"probe"}
	a.mu.Unlock()
	done := make(chan struct{})
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
				case <-time.After(100 * time.Millisecond):
					_ = cmd.Process.Signal(signal)
				case <-done:
				}
			}
		case <-done:
		case <-ctx.Done():
		}
	}
	<-done
	f.elapsed = time.Since(start)
	w.stdout, w.stderr, w.exit = out.String(), errout.String(), cmd.ProcessState.ExitCode()
	return nil // Missing launch, signal death and deadline are assertion observations.
}
