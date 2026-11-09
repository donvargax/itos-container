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

func (w *world) lifecycleLocalSteps(sc *godog.ScenarioContext) {
	var child int
	selection := "drain"
	var result doctorResult
	w.lifecycleTransitionSteps(sc)
	w.lifecycleBudgetSteps(sc)
	w.lifecycleRecoverySteps(sc)
	w.lifecycleTeardownReviewSteps(sc)
	sc.Step(`^a lifecycle contract fixture with observed exit (\d+) and a final output drain that never completes$`, func(n int) { child = n })
	sc.Step(`^a lifecycle readmission fixture with observed exit (\d+) and a stalled final output drain$`, func(n int) { child, selection = n, "readmit" })
	run := func() error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestLifecycleContract$", "-test.v", "-test.count=1"}
		if cover := w.coverageDir(); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(w.coverageEnv(os.Environ()), "SLICE_12_CONTRACT="+selection+strconv.Itoa(child))
		data, err := cmd.CombinedOutput()
		result = doctorResult{stdout: string(data)}
		if err != nil {
			result.exit = 1
		}
		return nil
	}
	sc.Step(`^run cleans the worker and controlled time crosses the 10 second drain deadline through each local backend$`, run)
	sc.Step(`^run finishes cleanup and a fresh run follows the stream timeout through each local backend$`, run)
	assert := func() error {
		if result.exit != 0 || !strings.Contains(result.stdout, "--- PASS: TestLifecycleContract ") {
			return fmt.Errorf("lifecycle contract failed: %s", result.stdout)
		}
		return nil
	}
	sc.Step(`^each run reports "run.stream-timeout" and returns (\d+) without waiting beyond the shared teardown deadline$`, func(_ int) error { return assert() })
	sc.Step(`^the authoritative workload exit remains (\d+) and confirmed owned cleanup remains complete$`, func(_ int) error { return assert() })
	sc.Step(`^available trailing bytes are preserved without storing workload output in diagnostics or recovery$`, assert)
	sc.Step(`^the fresh run launches and cleans its own worker successfully$`, assert)
	sc.Step(`^the original stopped session retains its observed exit and stream timeout without changing its completed cleanup$`, assert)
}
