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

func (w *world) lifecycleRecoverySteps(sc *godog.ScenarioContext) {
	var operation string
	var result doctorResult
	sc.Step(`^a lifecycle recovery fixture with a protected owned session and stalled "([^"]*)" observations$`, func(s string) { operation = s })
	sc.Step(`^"([^"]*)" exhausts its recovery budget and a fresh invocation follows after the fault clears through each local backend$`, func(command string) error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestLifecycleRecoveryContract$", "-test.v", "-test.count=1"}
		if cover := w.coverageDir(); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(w.coverageEnv(os.Environ()), "SLICE_12_RECOVERY="+operation, "SLICE_12_COMMAND="+command)
		data, err := cmd.CombinedOutput()
		result = doctorResult{stdout: string(data)}
		if err != nil {
			result.exit = 1
		}
		return nil
	})
	assert := func() error {
		if result.exit != 0 || !strings.Contains(result.stdout, "--- PASS: TestLifecycleRecoveryContract ") {
			return fmt.Errorf("lifecycle recovery contract failed: %s", result.stdout)
		}
		return nil
	}
	for _, phrase := range []string{
		`the first recovery invocation reports recovery.temporary with exit 75 within (?:10|30) seconds with every finite call bounded to the remaining budget or 10 seconds`,
		`the first result retains the session identity and uncertainty without claiming stopped resources or a workload exit`,
		`the fresh invocation gets a new (?:10|30) second budget and completes only its requested inspection or owned cleanup`,
		`neither invocation creates starts or adopts a worker or changes another session`,
	} {
		sc.Step("^"+phrase+"$", assert)
	}
}
