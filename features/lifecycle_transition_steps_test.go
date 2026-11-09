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

func (w *world) lifecycleTransitionSteps(sc *godog.ScenarioContext) {
	var name, arg string
	var result doctorResult
	sc.Step(`^a lifecycle contract fixture with accepted start and a withheld reply followed by "([^"]*)"$`, func(s string) { name, arg = "start-timeout", s })
	sc.Step(`^a lifecycle contract fixture with observed exit (\d+) and slow drain reconciliation removal and completion$`, func(n int) { name, arg = "teardown", strconv.Itoa(n) })
	sc.Step(`^a lifecycle contract fixture with a cancellation barrier "([^"]*)"$`, func(s string) { name, arg = "cancel", s })
	sc.Step(`^a lifecycle contract fixture with an accepted "([^"]*)" whose response and observations are withheld$`, func(s string) { name, arg = "accepted", s })
	sc.Step(`^a lifecycle contract fixture with a live owned worker and lost signal acknowledgement and exit observations$`, func() { name, arg = "lost-signal", "" })
	run := func(signal string) error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestLifecycleLocalContract$", "-test.v", "-test.count=1"}
		if cover := w.coverageDir(); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(w.coverageEnv(os.Environ()), "SLICE_12_LOCAL="+name, "SLICE_12_ARGUMENT="+arg, "SLICE_12_SIGNAL="+signal)
		data, err := cmd.CombinedOutput()
		result = doctorResult{stdout: string(data)}
		if err != nil {
			result.exit = 1
		}
		return nil
	}
	sc.Step(`^the start call expires and run reconciles and cleans through each local backend$`, func() error { return run("") })
	sc.Step(`^run begins teardown and controlled time reaches 30 seconds through each local backend$`, func() error { return run("") })
	sc.Step(`^the run contract receives "([^"]*)" at that barrier and the pending operation returns through each local backend$`, run)
	sc.Step(`^run receives SIGTERM in flight and returns before the operation becomes observable and a fresh controller later reconciles it$`, func() error { return run("SIGTERM") })
	sc.Step(`^run receives "([^"]*)" and controlled time crosses grace and teardown deadlines through each local backend$`, run)
	assert := func() error {
		if result.exit != 0 || !strings.Contains(result.stdout, "--- PASS: TestLifecycleLocalContract ") {
			return fmt.Errorf("lifecycle contract failed: %s", result.stdout)
		}
		return nil
	}
	for _, phrase := range []string{
		`each run reports the start call timeout separately and returns (?:17|75) with workload outcome "(?:17|unknown)"`,
		`exactly one create and one start were attempted and no late reply restarts or duplicates work`,
		`cleanup uses fresh ownership and full ID inspection and retains recovery when absence is unconfirmed`,
		`each run reports "run.teardown-timeout" by the original teardown deadline and returns (?:17|75)`,
		`the workload exit remains (?:0|17) with incomplete cleanup and retained recovery evidence`,
		`owned reconciliation begins while final output drain is still pending without discarding available trailing bytes`,
		`no drain progress retry reconnect or repeated cancellation restarts the teardown clock`,
		`no successful removal reply is reported as confirmed durable absence`,
		`each run reports "run.cancelled" with exit (?:130|143) and a known not-started workload`,
		`no operation belonging to a later startup phase or worker signal is attempted`,
		`any accepted unstarted worker is reconciled and cleaned within the separate teardown budget`,
		`the first run returns 143 with "run.cancelled" and workload outcome "(?:not-started|unknown)" while retaining the original protected intent`,
		`the first cleanup neither records unfenced absence nor retries create or start`,
		`the later controller reconciles the original full identity and cleans without launching or restarting work`,
		`no accepted resource or checkout edit is lost from the independent audit`,
		`each run preserves "run.cancelled" with exit (?:130|143) and an explicitly unknown workload outcome`,
		`the first signal is attempted once and escalation is attempted at the original 10 second deadline without a second grace period`,
		`each mutation revalidates the original daemon record and full ID and uncertain cleanup retains recovery`,
		`successful signal or removal acknowledgements never synthesize exit zero or 137`,
	} {
		sc.Step("^"+phrase+"$", assert)
	}
}
