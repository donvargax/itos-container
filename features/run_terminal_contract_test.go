package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// Reports come from a test-only command assembly. Features never imports a
// product package or supplies the exit code/diagnostic the command must return.
type terminalContractObservation struct {
	Status                                                                          map[string]any
	RecoveryRestarted                                                               bool
	Backend, Diagnostic, Output, Error                                              string
	Code                                                                            int
	Ready, Fired, TTY, InputRestored, OutputRestored, ObserversEnded, AcquiredInput bool
	Initial                                                                         struct{ Columns, Rows int }
	Result, Record, Recovery                                                        map[string]any
	Elapsed                                                                         time.Duration
	Events                                                                          []struct {
		Name, Target     string
		After, Remaining time.Duration
	}
	Remaining int
	Retry     *terminalContractObservation
}
type terminalContract struct {
	w                  *world
	kind, fault, child string
	reports            []terminalContractObservation
}

func (w *world) terminalContractSteps(sc *godog.ScenarioContext, bind func(*terminalContract)) {
	var f *terminalContract
	setup := func(kind, fault, child string) {
		f = &terminalContract{w: w, kind: kind, fault: fault, child: child}
		bind(f)
	}
	sc.Step(`^a terminal-run contract fixture with distinct terminal stdin and stdout$`, func() { setup("acquisition", "", "0") })
	sc.Step(`^the fixture schedules "([^"]*)" after stdin enters raw mode$`, func(fault string) { f.fault = fault })
	sc.Step(`^a terminal-run contract fixture with fault "([^"]*)" and authoritative exit "([^"]*)"$`, func(fault, child string) { setup("fault", fault, child) })
	sc.Step(`^a terminal-run contract fixture with blocked restoration diagnostics and authoritative exit "([^"]*)"$`, func(child string) { setup("diagnostic", "saved terminal restoration fails", child) })
	sc.Step(`^terminal run returns (\d+) within the remaining original teardown budget despite blocked restoration diagnostics$`, func(code int) error {
		return f.each(false, func(r terminalContractObservation) error {
			if !r.Ready || r.Code != code || r.Elapsed != 30*time.Second || !strings.Contains(r.Diagnostic, "run.terminal-restore") || r.Result["Cleanup"] != "cleanup-complete" {
				return fmt.Errorf("blocked restoration diagnostic exceeded original teardown or lost result: %+v", r)
			}
			return nil
		})
	})
	sc.Step(`^a terminal-run contract fixture with disconnect condition "([^"]*)" and authoritative exit "([^"]*)"$`, func(fault, child string) { setup("disconnect", fault, child) })
	sc.Step(`^terminal run attempts acquisition then retries after clearing the fault through the applicable local backends$`, func() error { return f.invoke() })
	sc.Step(`^terminal run reaches the fault and finishes through the applicable local backends alongside a healthy control$`, func() error { return f.invoke() })
	sc.Step(`^terminal run loses its client and later status and stop inspect the retained session after the fixture fault clears$`, func() error { return f.invoke() })
	sc.Step(`^the first attempt restores each changed terminal to its own exact saved state and ends all terminal observers$`, func() error {
		return f.each(false, func(r terminalContractObservation) error {
			if !r.AcquiredInput || !r.InputRestored || !r.OutputRestored || !r.ObserversEnded {
				return fmt.Errorf("partial acquisition did not roll back/finish: %+v", r)
			}
			return nil
		})
	})
	sc.Step(`^the retry starts with stdin size 113 by 37 delivers exact raw output and exits zero after owned cleanup$`, func() error {
		return f.each(false, func(r terminalContractObservation) error {
			if r.Retry == nil {
				return fmt.Errorf("missing same-terminal retry")
			}
			retry := r.Retry
			if !retry.Ready || !retry.TTY || retry.Initial.Columns != 113 || retry.Initial.Rows != 37 || retry.Code != 0 || retry.Output != "slice-37 contract raw\n\r\x01\x00payload" || retry.Result["Cleanup"] != "cleanup-complete" || retry.Remaining != 1 {
				return fmt.Errorf("retry failed: %+v", retry)
			}
			return nil
		})
	})
	sc.Step(`^both attempts leave both caller terminal handles usable with their exact original settings and approval intact$`, func() error {
		return f.each(false, func(r terminalContractObservation) error {
			if !r.InputRestored || !r.OutputRestored || r.Retry == nil || !r.Retry.InputRestored || !r.Retry.OutputRestored {
				return fmt.Errorf("retry lost caller terminal state: %+v", r)
			}
			return nil
		})
	})
	sc.Step(`^terminal run reports "([^"]*)" separately from workload outcome "([^"]*)" and returns (\d+)$`, func(rule, child string, result int) error { return f.outcome(rule, child, result) })
	sc.Step(`^the first terminal run reports "run.terminal-disconnected" with workload outcome "([^"]*)" and returns (\d+)$`, func(child string, result int) error { return f.outcome("run.terminal-disconnected", child, result) })
	sc.Step(`^terminal teardown restores or reports the failed restoration without waiting for input output or unread resize events$`, func() error {
		return f.each(false, func(r terminalContractObservation) error {
			if !r.ObserversEnded || r.Elapsed > 40*time.Second {
				return fmt.Errorf("terminal observation or restoration outlived bounded teardown: %+v", r)
			}
			if (!r.InputRestored || !r.OutputRestored) && !strings.Contains(r.Diagnostic, "run.terminal-restore") {
				return fmt.Errorf("restoration failure hidden: %+v", r)
			}
			return nil
		})
	})
	sc.Step(`^a failed restoration of one distinct terminal does not prevent exact restoration of the other usable terminal$`, func() error {
		return f.each(false, func(r terminalContractObservation) error {
			if strings.HasPrefix(f.fault, "distinct stdin") && !r.OutputRestored || strings.HasPrefix(f.fault, "distinct stdout") && !r.InputRestored {
				return fmt.Errorf("other terminal was not restored: %+v", r)
			}
			return nil
		})
	})
	sc.Step(`^owned stop drain and cleanup share the original budgets without duplicate signals or late resizes$`, func() error { return f.each(false, terminalContractBudgets) })
	sc.Step(`^the healthy terminal control returns zero restores exact settings and confirms owned cleanup$`, func() error {
		return f.each(true, func(r terminalContractObservation) error {
			if !r.Ready || !r.TTY || r.Code != 0 || !r.InputRestored || !r.OutputRestored || !r.ObserversEnded || r.Result["Cleanup"] != "cleanup-complete" || r.Remaining != 1 {
				return fmt.Errorf("healthy control failed: %+v", r)
			}
			return nil
		})
	})
	sc.Step(`^terminal run retains any cleanup failures and the original recovery identity without claiming an unobserved exit or unconfirmed absence$`, func() error {
		return f.each(false, func(r terminalContractObservation) error {
			if r.Result["SessionID"] == "" || r.Result["Reference"] == "" {
				return fmt.Errorf("recovery identity missing")
			}
			if f.child == "unknown" && r.Record["workload_exit"] != nil {
				return fmt.Errorf("invented a persisted workload exit")
			}
			if (f.fault == "owned removal fails" || f.fault == "engine becomes unreachable" || f.fault == "inspected ownership changes") && r.Result["Cleanup"] == "cleanup-complete" {
				return fmt.Errorf("uncertain cleanup was called complete")
			}
			return nil
		})
	})
	sc.Step(`^every signal resize and removal is confined to freshly verified ownership and no foreign resource is touched$`, func() error {
		return f.each(false, func(r terminalContractObservation) error {
			for _, event := range r.Events {
				if (event.Name == "resize" || event.Name == "remove" || strings.HasPrefix(event.Name, "SIG")) && event.Target != r.Result["ID"] {
					return fmt.Errorf("operation targeted another resource: %+v", event)
				}
			}
			return terminalContractBudgets(r)
		})
	})
	sc.Step(`^the later recovery reports actual ownership and absence without restarting work or rewriting unknown history as success$`, func() error {
		return f.each(false, func(r terminalContractObservation) error {
			entries, _ := r.Status["Entries"].([]any)
			if r.RecoveryRestarted || len(entries) != 1 {
				return fmt.Errorf("fresh status/stop lost or restarted the session: %+v", r)
			}
			failures, _ := r.Recovery["Failures"].([]any)
			if f.fault == "inspected ownership changes" {
				if r.Remaining != 2 || len(failures) == 0 {
					return fmt.Errorf("foreign identity was adopted: %+v", r)
				}
				return nil
			}
			if r.Remaining != 1 || len(failures) != 0 {
				return fmt.Errorf("recovery failed after fault cleared: %+v", r)
			}
			return nil
		})
	})
}

func (f *terminalContract) invoke() error {
	bin, err := f.w.sessionProbeBinary()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestTerminalRunContract$", "-test.count=1"}
	if cover := f.w.coverageDir(); cover != "" {
		args = append(args, "-test.gocoverdir="+cover)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir, cmd.Env = f.w.dir, f.w.env()
	for key, value := range map[string]string{"SLICE_37_CONTRACT": f.kind, "SLICE_37_FAULT": f.fault, "SLICE_37_CHILD": f.child} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	data, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("terminal contract process: %w\n%s", err, data)
	}
	if err := json.Unmarshal(data, &f.reports); err != nil {
		return fmt.Errorf("terminal contract transcript: %w\n%s", err, data)
	}
	return nil
}

func (f *terminalContract) each(healthy bool, check func(terminalContractObservation) error) error {
	want := 4
	if f.kind == "acquisition" {
		want = 6
	}
	if len(f.reports) != want {
		return fmt.Errorf("missing contract backends/controls: %+v", f.reports)
	}
	start := 0
	if healthy {
		start = 1
	}
	for i := start; i < len(f.reports); i += 2 {
		r := f.reports[i]
		if r.Error != "" {
			return fmt.Errorf("%s: %s", r.Backend, r.Error)
		}
		if err := check(r); err != nil {
			return fmt.Errorf("%s: %w", r.Backend, err)
		}
	}
	return nil
}

func (f *terminalContract) firstRefusal(rule string, code int) error {
	return f.each(false, func(r terminalContractObservation) error {
		if r.Code != code || !strings.Contains(r.Diagnostic, rule) {
			return fmt.Errorf("first acquisition result differs: %+v", r)
		}
		for _, e := range r.Events {
			if e.Name == "preflight" || e.Name == "open" || e.Name == "create" {
				return fmt.Errorf("acquisition refusal reached startup: %+v", e)
			}
		}
		return nil
	})
}

func (f *terminalContract) outcome(rule, child string, result int) error {
	return f.each(false, func(r terminalContractObservation) error {
		if !r.Ready || r.Code != result || !strings.Contains(r.Diagnostic, rule) {
			return fmt.Errorf("terminal fault result differs: %+v", r)
		}
		if child == "unknown" {
			if r.Result["Exited"] == true {
				return fmt.Errorf("unknown child became an exit")
			}
		} else {
			code, _ := strconv.Atoi(child)
			if r.Result["Exited"] != true || r.Result["Exit"] != float64(code) {
				return fmt.Errorf("observed child exit was lost: %+v", r.Result)
			}
		}
		return nil
	})
}

func terminalContractBudgets(r terminalContractObservation) error {
	counts := map[string]int{}
	teardown := false
	var firstSignal time.Duration
	for _, e := range r.Events {
		if strings.HasPrefix(e.Name, "SIG") || e.Name == "remove" {
			counts[e.Name]++
			teardown = true
		}
		if e.Name == "SIGTERM" {
			firstSignal = e.After
		}
		if e.Name == "SIGKILL" && e.After-firstSignal != 10*time.Second {
			return fmt.Errorf("grace moved: %+v", r.Events)
		}
		if teardown && e.Name == "resize" {
			return fmt.Errorf("resize after teardown")
		}
		if e.Name == "resize" && (e.Remaining <= 0 || e.Remaining > 10*time.Second) {
			return fmt.Errorf("unbounded resize")
		}
	}
	if counts["SIGTERM"] > 1 || counts["SIGKILL"] > 1 || counts["remove"] > 1 {
		return fmt.Errorf("duplicate termination/removal: %+v", r.Events)
	}
	if r.Elapsed > 40*time.Second {
		return fmt.Errorf("terminal lifecycle exceeded call plus teardown budgets")
	}
	return nil
}
