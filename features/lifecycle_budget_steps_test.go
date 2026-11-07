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

func (w *world) lifecycleBudgetSteps(sc *godog.ScenarioContext) {
	var name, operation string
	var result doctorResult
	sc.Step(`^a lifecycle exited worker with an unrecorded exit and a cancellation-ignoring recovery wait$`, func() {
		name, operation = "teardown wait", ""
	})
	sc.Step(`^a lifecycle launch with a cancellation-ignoring "([^"]*)" interrupted by "([^"]*)"$`, func(op, cause string) {
		name, operation = "startup lifetime", cause+"|"+op
	})
	sc.Step(`^a lifecycle contract fixture with a stalled "([^"]*)" and a healthy control$`, func(s string) { name, operation = "call", s })
	sc.Step(`^a lifecycle contract fixture with 115 seconds of successful preflight and publication progress and delayed launch observations$`, func() { name, operation = "startup", "" })
	sc.Step(`^a lifecycle checkout with a blocked "([^"]*)" interrupted by "([^"]*)"$`, func(op, cause string) {
		name, operation = "checkout lifetime", cause+"|"+op
	})
	run := func() error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestLifecycleBudgetContract$", "-test.v", "-test.count=1"}
		if cover := os.Getenv("GOCOVERDIR"); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(), "SLICE_12_BUDGET="+name, "SLICE_12_ARGUMENT="+operation)
		data, err := cmd.CombinedOutput()
		result = doctorResult{stdout: string(data)}
		if err != nil {
			result.exit = 1
		}
		return nil
	}
	sc.Step(`^the real run contract reaches the operation and controlled time advances through its 10 second deadline and the ensuing cleanup through each local backend$`, run)
	sc.Step(`^run proceeds through the full invocation and controlled time advances through the 120 second startup deadline and the ensuing cleanup through each local backend$`, run)
	sc.Step(`^run stops waiting and the abandoned checkout operation later finishes through each local backend$`, run)
	sc.Step(`^run returns before the pending launch operation is released through each local backend$`, run)
	sc.Step(`^run returns before the pending exit observation is released through each local backend$`, run)
	assert := func() error {
		if result.exit != 0 || !strings.Contains(result.stdout, "--- PASS: TestLifecycleBudgetContract ") {
			return fmt.Errorf("lifecycle budget contract failed: %s", result.stdout)
		}
		return nil
	}
	for _, phrase := range []string{
		`each run cancels its wait for the pending operation by its call deadline and begins no subsequent startup phase`,
		`any accepted mutation remains accountable to the original intent`,
		`after the cleanup attempt each run reports "run.call-timeout" with exit 75 and the named operation no later than 30 seconds after its call deadline`,
		`the healthy control launches and cleans exactly one owned worker with unchanged unrelated state`,
		`each run cancels its wait for the pending startup operation at the original 120 second deadline and begins no subsequent startup phase`,
		`progress reconnects and any retries never extend startup or begin a phase after the deadline`,
		`the pending operation receives only the remaining startup time and cleanup receives its own 30 second budget`,
		`after the cleanup attempt each run reports "run.startup-timeout" with exit 75 no later than 30 seconds after the original startup deadline`,
		`delayed replies cannot launch work or replace the reported timeout`,
		`run returns its bounded interruption result without closing checkout state used by the pending operation`,
		`the late completion releases its checkout pins without launching a worker or replacing the result`,
		`run reports the interruption within its original operation and teardown budgets while retaining the pending connection`,
		`late launch completion closes its connection and any attachment without resuming launch retrying mutations or changing the returned result`,
		`run reports its original failure and teardown timeout within 30 seconds with unknown workload outcome and retained recovery`,
		`the pending exit observation retains its connection until completion without changing the returned result or recovery record`,
		`a healthy foreground wait longer than startup still returns its observed exit and cleans only its owned worker`,
	} {
		sc.Step("^"+phrase+"$", assert)
	}
}
