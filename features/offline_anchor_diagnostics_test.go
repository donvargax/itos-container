package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) anchorDiagnosticSteps(sc *godog.ScenarioContext) {
	var mode string
	var result struct {
		Run                runExitCommand
		Expected, Observed string
		Events             []string
		Workers, Anchors   int
	}
	sc.Step(`^an anchor diagnostic fixture with "([^"]*)" failure$`, func(value string) { mode = value })
	sc.Step(`^the registered command encounters that mount observation failure$`, func() error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestPublicAnchorDiagnosticContract$", "-test.count=1", "-test.v"}
		if cover := w.coverageDir(); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(w.coverageEnv(os.Environ()), "SLICE_17_DIAGNOSTIC="+mode)
		data, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("diagnostic fixture: %w\n%s", err, data)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if encoded, ok := strings.CutPrefix(line, "SLICE_17_DIAGNOSTIC_RESULT="); ok {
				return json.Unmarshal([]byte(encoded), &result)
			}
		}
		return fmt.Errorf("missing diagnostic observation: %s", data)
	})
	sc.Step(`^the mount refusal identifies "([^"]*)" and the expected identity with the observed identity or syscall errno$`, func(check string) error {
		want := []string{"run.mount-refused", "check=" + check, "expected=" + result.Expected}
		switch mode {
		case "device", "inode":
			want = append(want, "observed="+result.Observed)
		case "stat":
			want = append(want, "errno=5", "input/output error")
		case "pidfd":
			want = append(want, "errno=3", "no such process")
		case "invalid PID":
			want = append(want, "pid=0")
		}
		for _, text := range want {
			if !strings.Contains(result.Run.Stderr, text) {
				return fmt.Errorf("missing %q in mount refusal: %+v", text, result.Run)
			}
		}
		return nil
	})
	sc.Step(`^diagnostic detail preserves refusal and immediate owned cleanup without private error paths or workload arguments$`, func() error {
		if result.Run.Exit != 1 || result.Run.Stdout != "" || result.Workers != 0 || result.Anchors != 0 || strings.Contains(result.Run.Stderr, "SYNTHETIC_PRIVATE") {
			return fmt.Errorf("unsafe or behavior-changing diagnostics: %+v", result)
		}
		return runExitOrder(runExitObservation{Events: result.Events}, "start", "SIGKILL", "remove", "anchor remove")
	})
}
