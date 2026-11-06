package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// The memory route runs the same public commands with stateful port fakes in
// cmd, the sole adapter assembly boundary. It independently checks the full
// local contract matrix; the filesystem route checks the current example and
// OS effects. Capture a missing/wrong build as data, then fail at the first Then.
func (w *world) recoveryFakeBackendSteps(sc *godog.ScenarioContext) {
	var pending *doctorResult
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		pending = nil
		return ctx, nil
	})
	sc.StepContext().After(func(ctx context.Context, step *godog.Step, _ godog.StepResultStatus, stepErr error) (context.Context, error) {
		if pending != nil {
			r := pending
			pending = nil
			if stepErr == nil && (r.exit != 0 || !strings.Contains(r.stdout, "--- PASS: TestPublicRecoveryFakeBackend ")) {
				return ctx, fmt.Errorf("memory command backend did not satisfy recovery behavior: exit %d\n%s", r.exit, r.stdout)
			}
		}
		if stepErr != nil || !strings.Contains(step.Text, "through each local backend") {
			return ctx, nil
		}
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return ctx, err
		}
		run, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestPublicRecoveryFakeBackend$", "-test.count=1", "-test.v"}
		if cover := os.Getenv("GOCOVERDIR"); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		command := exec.CommandContext(run, bin, args...)
		data, err := command.CombinedOutput()
		r := doctorResult{stdout: string(data)}
		if err != nil {
			r.exit = 1
		}
		pending = &r
		return ctx, nil
	})
}
