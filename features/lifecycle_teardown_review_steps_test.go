package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) lifecycleTeardownReviewSteps(sc *godog.ScenarioContext) {
	var selection string
	var result doctorResult
	sc.Step(`^a lifecycle teardown fixture with observed exit (\d+) and "([^"]*)" diagnostic output$`, func(child int, sink string) {
		selection = "diagnostics:" + strconv.Itoa(child) + ":" + sink
	})
	sc.Step(`^a lifecycle teardown fixture with "([^"]*)" followed by stalled "([^"]*)"$`, func(cause, phase string) {
		selection = "loss:" + cause + ":" + phase
	})
	sc.Step(`^a lifecycle output fixture with observed exit (\d+) and "([^"]*)" diagnostics after a stalled final drain$`, func(child int, sink string) {
		selection = "output:" + strconv.Itoa(child) + ":" + sink
	})
	run := func() error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestLifecycleTeardownReview$", "-test.v", "-test.count=1"}
		if cover := w.coverageDir(); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(w.coverageEnv(os.Environ()), "SLICE_12_REVIEW="+selection)
		data, err := cmd.CombinedOutput()
		result = doctorResult{stdout: string(data)}
		if err != nil {
			result.exit = 1
		}
		return nil
	}
	sc.Step(`^run exhausts cleanup and its return-time diagnostics are captured through each local backend$`, run)
	sc.Step(`^run finishes owned cleanup and attempts diagnostics through each local backend$`, run)
	assert := func() error {
		if result.exit != 0 || !strings.Contains(result.stdout, "--- PASS: TestLifecycleTeardownReview ") {
			return fmt.Errorf("teardown review contract failed: %s", result.stdout)
		}
		return nil
	}
	for _, phrase := range []string{
		`writable diagnostics contain run.teardown-timeout and the exact retained session retry before run returns`,
		`blocked diagnostics cannot extend the original 30 second teardown deadline`,
		`the original run failure is reported separately from run.teardown-timeout`,
		`observed workload exits keep precedence and missing observations remain unknown`,
		`fresh recovery cleans only the retained owned worker without restarting it`,
		`diagnostic output waits at most 10 seconds within the original teardown deadline`,
		`a diagnostic sink writable before its call deadline receives the stream timeout before run returns`,
		`the workload exit and durable cleanup failure history survive blocked or late diagnostics`,
		`a readable diagnostic control preserves normal completion and unrelated state`,
	} {
		sc.Step("^"+phrase+"$", assert)
	}
}
