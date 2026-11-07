package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type runExitCommand struct {
	Exit           int
	Stdout, Stderr string
}

type runExitObservation struct {
	Run, Stop, Repeated, Status                     runExitCommand
	CompletedRecord, FinalRecord                    string
	CompletedIntent, FinalIntent                    string
	MutationsAtCompletion, MutationsAfter           int
	Events                                          []string
	ProbeSeconds, CompletionSeconds, ElapsedSeconds int
	DrainBudgetSeconds                              int
}

func (w *world) runExitSteps(sc *godog.ScenarioContext) {
	var schedule string
	var results map[string]runExitObservation
	sc.Step(`^an unknown-exit run fixture with stop completion "([^"]*)"$`, func(value string) {
		schedule, results = value, nil
	})
	sc.Step(`^the fixture runs the foreground command and concurrent stop then repeats stop and inspects JSON status$`, func() error {
		bin, err := w.sessionProbeBinary()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-test.run=^TestPublicRunExitContract$", "-test.count=1", "-test.v"}
		if cover := os.Getenv("GOCOVERDIR"); cover != "" {
			args = append(args, "-test.gocoverdir="+cover)
		}
		command := exec.CommandContext(ctx, bin, args...)
		command.Env = append(os.Environ(), "SLICE_24_SCHEDULE="+schedule)
		data, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("run-exit fixture: %w\n%s", err, data)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if encoded, ok := strings.CutPrefix(line, "SLICE_24_RESULT="); ok {
				return json.Unmarshal([]byte(encoded), &results)
			}
		}
		return fmt.Errorf("missing run-exit observations: %s", data)
	})
	sc.Step(`^the foreground run returns wrapper exit 75 for the unobserved workload exit$`, func() error {
		r, ok := results["unknown"]
		if !ok || !slices.Contains(r.Events, "run-wait-canceled") || !slices.Contains(r.Events, "completion-published") {
			return fmt.Errorf("run did not reach unknown-exit reconciliation: %+v", r)
		}
		if r.Run.Exit != 75 {
			return fmt.Errorf("foreground exit = %d, want wrapper 75; stderr %q", r.Run.Exit, r.Run.Stderr)
		}
		return nil
	})
	sc.Step(`^run reports run.workload-exit-unknown with completed cleanup and no cleanup failure or retry advice$`, func() error {
		diagnostic := results["unknown"].Run.Stderr
		for _, want := range []string{"run.workload-exit-unknown", "workload exit is unknown", "cleanup is complete"} {
			if !strings.Contains(diagnostic, want) {
				return fmt.Errorf("diagnostic lacks %q: %q", want, diagnostic)
			}
		}
		for _, forbidden := range []string{"run.cleanup-", "run.execution-unavailable", "itos container stop", "retry", "rerun", "cleanup is not confirmed"} {
			if strings.Contains(diagnostic, forbidden) {
				return fmt.Errorf("misleading %q in diagnostic: %q", forbidden, diagnostic)
			}
		}
		return nil
	})
	sc.Step(`^run observes stop completion at controlled second 11 after its initial 10 second probe expires within the existing teardown total$`, func() error {
		r := results["unknown"]
		if r.ProbeSeconds != 10 || r.CompletionSeconds != 11 || r.ElapsedSeconds != 11 {
			return fmt.Errorf("probe/completion/total seconds = %d/%d/%d", r.ProbeSeconds, r.CompletionSeconds, r.ElapsedSeconds)
		}
		return runExitOrder(r, "stop-locked", "stdin-closed", "run-wait-canceled", "probe-start", "probe-expired", "reconcile-start", "completion-published", "reconcile-acquired", "drain", "run-returned")
	})
	sc.Step(`^both stops and fresh status succeed with the same stopped absent session cleanup complete workload exit null and retry null$`, func() error {
		return runExitRecovery(results["unknown"], nil)
	})
	sc.Step(`^run and repeated recovery preserve the completed record bytes and absent intent without further engine mutations$`, func() error {
		return runExitPreserved(results["unknown"], nil)
	})
	sc.Step(`^run drains each trailing marker once on its original stdout or stderr stream$`, func() error {
		r := results["unknown"]
		if schedule == "before stdin reconciliation" {
			if err := runExitOrder(r, "completion-published", "stop-returned", "stdin-closed", "run-wait-canceled", "probe-start", "probe-acquired", "drain", "run-returned"); err != nil {
				return err
			}
		}
		return runExitDrained(r)
	})
	sc.Step(`^observed-exit and forwarding-failure controls retain their specified result precedence$`, func() error {
		for _, control := range []struct {
			name         string
			exit         int
			known        bool
			drainFailure bool
		}{{"zero", 0, true, false}, {"seven", 7, true, false}, {"seventy-five", 75, true, false}, {"unknown-drain", 75, false, true}, {"seven-drain", 7, true, true}} {
			r, ok := results[control.name]
			if !ok || r.Run.Exit != control.exit || strings.Contains(r.Run.Stderr, "run.workload-exit-unknown") {
				return fmt.Errorf("%s lost result precedence: %+v", control.name, r.Run)
			}
			if strings.Contains(r.Run.Stderr, "run.execution-unavailable") != control.drainFailure {
				return fmt.Errorf("%s lost forwarding diagnostic: %q", control.name, r.Run.Stderr)
			}
			if !control.drainFailure && r.Run.Stderr != "slice-24-trailing-stderr\n" {
				return fmt.Errorf("%s unexpected diagnostic: %q", control.name, r.Run.Stderr)
			}
			var exit *int
			if control.known {
				exit = &control.exit
			}
			for _, check := range []func() error{
				func() error { return runExitRecovery(r, exit) },
				func() error { return runExitPreserved(r, exit) },
				func() error { return runExitDrained(r) },
			} {
				if err := check(); err != nil {
					return fmt.Errorf("%s: %w", control.name, err)
				}
			}
		}
		return nil
	})
}

func runExitOrder(r runExitObservation, names ...string) error {
	previous := -1
	for _, name := range names {
		index := slices.Index(r.Events, name)
		if index <= previous {
			return fmt.Errorf("missing or out-of-order %q in %v", name, r.Events)
		}
		previous = index
	}
	return nil
}

func runExitDrained(r runExitObservation) error {
	if r.Run.Stdout != "slice-24-trailing-stdout\n" || strings.Count(r.Run.Stderr, "slice-24-trailing-stderr\n") != 1 || strings.Contains(r.Run.Stderr, "slice-24-trailing-stdout") || r.DrainBudgetSeconds <= 0 || r.DrainBudgetSeconds > 10 {
		return fmt.Errorf("lost or mixed trailing output / drain budget: %+v, %d seconds", r.Run, r.DrainBudgetSeconds)
	}
	return runExitOrder(r, "completion-published", "stop-returned", "drain", "run-returned")
}

func runExitRecovery(r runExitObservation, exit *int) error {
	id := ""
	for _, command := range []runExitCommand{r.Stop, r.Repeated, r.Status} {
		var report struct {
			Schema   int
			OK       bool
			Problems []json.RawMessage
			Sessions []struct {
				ID, State, Resource, Cleanup string
				WorkloadExit                 *int `json:"workload_exit"`
				Retry                        json.RawMessage
			}
		}
		if err := json.Unmarshal([]byte(command.Stdout), &report); err != nil {
			return err
		}
		if command.Exit != 0 || command.Stderr != "" || report.Schema != 1 || !report.OK || report.Problems == nil || len(report.Problems) != 0 || len(report.Sessions) != 1 {
			return fmt.Errorf("cleanup report not successful: %+v", command)
		}
		e := report.Sessions[0]
		if id == "" {
			id = e.ID
		}
		if len(id) != 64 || e.ID != id || e.State != "stopped" || e.Resource != "absent" || e.Cleanup != "complete" || string(e.Retry) != "null" || !runExitEqual(e.WorkloadExit, exit) {
			return fmt.Errorf("cleanup report changed session or result: %s", command.Stdout)
		}
	}
	return nil
}

func runExitEqual(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func runExitPreserved(r runExitObservation, exit *int) error {
	if r.CompletedRecord == "" || r.CompletedRecord != r.FinalRecord || r.CompletedIntent == "" || r.CompletedIntent != r.FinalIntent || r.MutationsAtCompletion != 4 || r.MutationsAfter != r.MutationsAtCompletion {
		return fmt.Errorf("completed authority or engine mutations changed: %+v", r)
	}
	var record struct {
		State        string
		WorkloadExit *int            `json:"workload_exit"`
		LastProblem  json.RawMessage `json:"last_problem"`
	}
	var intent struct {
		Stage       string
		ContainerID string `json:"container_id"`
	}
	if err := json.Unmarshal([]byte(r.FinalRecord), &record); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(r.FinalIntent), &intent); err != nil {
		return err
	}
	if record.State != "stopped" || !runExitEqual(record.WorkloadExit, exit) || string(record.LastProblem) != "null" || intent.Stage != "absent" || len(intent.ContainerID) != 64 {
		return fmt.Errorf("completion invented a workload result or failure: %s / %s", r.FinalRecord, r.FinalIntent)
	}
	return nil
}
