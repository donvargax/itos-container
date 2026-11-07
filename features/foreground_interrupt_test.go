package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) foregroundInterruptSteps(sc *godog.ScenarioContext) {
	var phase, outcome string
	var observed struct {
		Code       int
		Diagnostic string
		Cleaned    bool
		Started    bool
	}
	sc.Step(`^a foreground interrupt contract with "([^"]*)" at "([^"]*)"$`, func(failure, boundary string) {
		outcome, phase = failure, boundary
	})
	sc.Step(`^the foreground interrupt contract runs$`, func() error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		output := filepath.Join(w.dir, "slice-10-interrupt.json")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestForegroundInterruptContract$", "-test.count=1"}
		if cover := os.Getenv("GOCOVERDIR"); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(w.env(), "SLICE_10_INTERRUPT_OUTPUT="+output, "SLICE_10_INTERRUPT_PHASE="+phase, "SLICE_10_INTERRUPT_OUTCOME="+outcome)
		transcript, err := cmd.CombinedOutput()
		if err != nil {
			observed.Diagnostic = string(transcript)
			return nil
		}
		data, err := os.ReadFile(output)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &observed)
	})
	sc.Step(`^the foreground interrupt contract retains "([^"]*)" with exit 1 and owned cleanup$`, func(failure string) error {
		if observed.Code != 1 || !strings.Contains(observed.Diagnostic, "run."+failure+" ") || !observed.Cleaned || observed.Started != (phase == "wait") {
			return fmt.Errorf("interrupt replaced the classified refusal or lost cleanup: %+v", observed)
		}
		return nil
	})
	var aborted, signaled bool
	sc.Step(`^the run preflight is held at fresh image inspection$`, func() {
		w.doctor.engine.paused = true
	})
	sc.Step(`^the run receives "([^"]*)" during preflight$`, func(name string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, w.bin, "run", "--", "probe")
		cmd.Dir, cmd.Env = w.doctor.cwd, w.env()
		var out, errout strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &errout
		if err := cmd.Start(); err != nil {
			return err
		}
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-w.doctor.engine.reached:
			signal := syscall.SIGINT
			if name == "SIGTERM" {
				signal = syscall.SIGTERM
			}
			if err := cmd.Process.Signal(signal); err != nil {
				return err
			}
			signaled = true
			select {
			case <-done:
				expected := 130
				if name == "SIGTERM" {
					expected = 143
				}
				aborted = cmd.ProcessState.ExitCode() == expected && strings.Contains(errout.String(), "run.cancelled "+name) && strings.Contains(errout.String(), "workload outcome not-started")
			case <-time.After(time.Second):
			}
		case <-done:
		case <-ctx.Done():
		}
		w.doctor.engine.unpause()
		<-done
		w.stdout, w.stderr, w.exit = out.String(), errout.String(), cmd.ProcessState.ExitCode()
		return nil
	})
	sc.Step(`^preflight reports cancellation and aborts before release without a snapshot or worker$`, func() error {
		if !signaled || !aborted {
			return fmt.Errorf("host signal did not abort the held preflight: %s", w.report())
		}
		for _, name := range []string{"snapshots", "intents"} {
			path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", name)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				return fmt.Errorf("interrupted preflight published %s", name)
			}
		}
		for _, r := range w.doctor.engine.requests() {
			if r.Method != "GET" && r.Method != "HEAD" {
				return fmt.Errorf("interrupted preflight mutated the engine: %+v", r)
			}
		}
		return nil
	})
}
