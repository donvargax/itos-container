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

func (w *world) recoveryReviewSteps(sc *godog.ScenarioContext) {
	var fixture string
	var result doctorResult
	sc.Step(`^a recovery command fixture for "([^"]*)"$`, func(name string) {
		fixture = name
		result = doctorResult{}
	})
	sc.Step(`^the recovery command fixture exercises its configured interruption$`, func() error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestPublicRecoveryReviewContract$", "-test.count=1", "-test.v"}
		if cover := os.Getenv("GOCOVERDIR"); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		command := exec.CommandContext(ctx, bin, args...)
		command.Env = append(os.Environ(), "SLICE_11_REVIEW_CASE="+fixture)
		data, err := command.CombinedOutput()
		result.stdout = string(data)
		if err != nil {
			result.exit = 1
		}
		return nil
	})
	assert := func() error {
		if result.exit != 0 || !strings.Contains(result.stdout, "--- PASS: TestPublicRecoveryReviewContract ") {
			return fmt.Errorf("recovery interruption behavior failed: %s", result.stdout)
		}
		return nil
	}
	sc.Step(`^stop reports recovery.temporary with exit 75 within its original total and preserves ownership for a successful retry$`, assert)
	sc.Step(`^stop completes owned termination and removal with controller unknown while status retains the probe problem and foreign resources survive$`, assert)
	sc.Step(`^run finishes without a wait failure and status retains stopped cleanup with unknown workload exit and no repeated mutation$`, assert)
}
