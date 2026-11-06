package features

import (
	"context"
	"encoding/binary"
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
	started, exit chan struct{}
	finish        sync.Once
	signals       []string
	elapsed       time.Duration
}

func (w *world) foregroundRecoverySteps(sc *godog.ScenarioContext) {
	sc.Step(`^a foreground recovery fixture for "([^"]*)"$`, func(fault string) {
		w.localOfflineFixture()
		w.doctor.engine.offline.recovery = &foregroundRecovery{fault: fault, block: fault == "stdout" || fault == "stderr" || strings.Contains(fault, "SIG"), started: make(chan struct{}), exit: make(chan struct{})}
	})
	sc.Step(`^the foreground recovery command runs$`, w.foregroundRecoveryRun)
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
	case "replaced git", "missing git":
		git := filepath.Join(a.w.doctor.checkout, ".git")
		a.problem = os.Rename(git, git+".before-run")
		if a.problem == nil && f.fault == "replaced git" {
			a.problem = os.Mkdir(git, 0700)
		}
		_ = conn.Close()
	}
	if !f.block {
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
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, "run", "--", "probe")
	cmd.Dir, cmd.Env = w.doctor.cwd, w.env()
	var out, errout strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errout
	cmd.WaitDelay = time.Second
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
