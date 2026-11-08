package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type anchorExitObservation struct {
	Run, JSONStatus, TextStatus runExitCommand
	Record                      string
	Events                      []string
	Workers, Anchors            int
}

func (w *world) anchorExitSteps(sc *godog.ScenarioContext) {
	var point string
	var exit int
	var result anchorExitObservation
	sc.Step(`^an anchor detector fixture with exit (\d+) at "([^"]*)"$`, func(n int, p string) { exit, point = n, p })
	sc.Step(`^the registered command runs and fresh status reads its completed record$`, func() error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestPublicAnchorExitContract$", "-test.count=1", "-test.v"}
		if cover := os.Getenv("GOCOVERDIR"); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		command := exec.CommandContext(ctx, bin, args...)
		command.Env = append(os.Environ(), "SLICE_17_EXIT_POINT="+point, "SLICE_17_EXIT="+strconv.Itoa(exit))
		data, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("anchor exit fixture: %w\n%s", err, data)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if encoded, ok := strings.CutPrefix(line, "SLICE_17_EXIT_RESULT="); ok {
				return json.Unmarshal([]byte(encoded), &result)
			}
		}
		return fmt.Errorf("missing anchor exit observations: %s", data)
	})
	sc.Step(`^the command returns workload exit (\d+) without a mount refusal and completes owned cleanup$`, func(n int) error {
		var record struct {
			WorkloadExit *int `json:"workload_exit"`
			State        string
		}
		if err := json.Unmarshal([]byte(result.Record), &record); err != nil {
			return err
		}
		if result.Run.Exit != n || strings.Contains(result.Run.Stderr, "run.mount-") || record.WorkloadExit == nil || *record.WorkloadExit != n || record.State != "stopped" || result.Workers != 0 || result.Anchors != 0 {
			return fmt.Errorf("short-lived result lost: %+v", result)
		}
		return nil
	})
	sc.Step(`^the durable record and JSON and text status report "([^"]*)" for the mount detector$`, func(want string) error {
		var record map[string]any
		var status struct{ Sessions []map[string]any }
		if err := json.Unmarshal([]byte(result.Record), &record); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(result.JSONStatus.Stdout), &status); err != nil {
			return err
		}
		if record["mount_detector"] != want || len(status.Sessions) != 1 || status.Sessions[0]["mount_detector"] != want || result.JSONStatus.Exit != 0 || result.TextStatus.Exit != 0 || !strings.Contains(result.TextStatus.Stdout, "mount_detector "+want) {
			return fmt.Errorf("missing truthful durable detector observation: %+v", result)
		}
		return nil
	})
	sc.Step(`^the detector releases every process handle and never signals the exited worker$`, func() error {
		opens, closes := 0, 0
		for _, e := range result.Events {
			if strings.HasPrefix(e, "pidfd-open:") {
				opens++
			}
			if e == "pidfd-close" {
				closes++
			}
			if e == "SIGKILL" || e == "SIGTERM" {
				return fmt.Errorf("signaled exited worker: %v", result.Events)
			}
		}
		if opens != closes || (point == "between inspect and stat" && opens != 1) {
			return fmt.Errorf("process handle lifetime: %v", result.Events)
		}
		return nil
	})
	sc.Step(`^the detector pins the inspected PID and refuses its changed identity before reading any mount$`, func() error {
		if result.Run.Exit == 0 || !strings.Contains(result.Run.Stderr, "run.mount-refused") || slices.Contains(result.Events, "mounted-stat") {
			return fmt.Errorf("reused PID supplied mount authority: %+v", result)
		}
		return runExitOrder(runExitObservation{Events: result.Events}, "inspect:123", "pidfd-open:123", "inspect:456", "pidfd-close")
	})
	sc.Step(`^the detector kills only the revalidated owned worker and completes worker then anchor cleanup$`, func() error {
		if result.Workers != 0 || result.Anchors != 0 || slices.Contains(result.Events, "SIGTERM") {
			return fmt.Errorf("incomplete immediate teardown: %+v", result)
		}
		return runExitOrder(runExitObservation{Events: result.Events}, "pidfd-close", "SIGKILL", "remove", "anchor remove")
	})
}
