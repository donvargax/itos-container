package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type recoveryLocal struct {
	args                                 []string
	results                              []doctorResult
	before                               map[string]string
	watch                                *sessionReadWatch
	reads                                []string
	id, phase, resource, exit, mode      string
	immutable                            map[string]string
	ignoreTERM                           bool
	unavailable, malformed, stall        bool
	processes                            []*recoveryProcess
	fault, boundary                      string
	workerExit                           chan struct{}
	runExit                              int
	keepRunning, failRemove, lostRemove  bool
	removeReady, removeRelease           chan struct{}
	beforeRequests                       int
	lease, unrelated                     *recoveryProbe
	unavailableIDs                       map[string]bool
	runFixture                           bool
	probes                               []*recoveryProbe
	barrierReached                       bool
	registrations                        []recoveryRegistration
	markers                              []string
	mutation, substitution               string
	inspectionCount                      int
	inspectReady, inspectRelease         chan struct{}
	afterRemoveReady, afterRemoveRelease chan struct{}
	second                               *world
}

func (w *world) recoveryLocalSteps(sc *godog.ScenarioContext) {
	w.recoveryStateSteps(sc)
	w.recoveryFaultSteps(sc)
	w.recoveryDiscoverySteps(sc)
	w.recoveryRunSteps(sc)
	w.recoveryRaceSteps(sc)
	sc.Step(`^a recovery fixture with no engine access and checkout helper sentinels$`, w.recoveryInvocationFixture)
	sc.Step(`^product status help and stop help are requested$`, func() error {
		for _, command := range []string{"status", "stop"} {
			if err := w.recoveryInvocation(command, "--help"); err != nil {
				return err
			}
		}
		return w.recoveryFinishWatch()
	})
	sc.Step(`^both help requests return zero and describe their exact arguments JSON project scope and exit meanings$`, w.recoveryHelp)
	sc.Step(`^help explains that stop confirms cleanup rather than workload success and controller absence does not authorize restart$`, func() error {
		for _, r := range w.recoveryLocal.results {
			for _, text := range []string{"cleanup", "workload", "controller", "restart"} {
				if !strings.Contains(r.stdout, text) {
					return fmt.Errorf("recovery help omits %s", text)
				}
			}
		}
		return nil
	})
	sc.Step(`^help performs no state access engine connection approval or checkout helper execution$`, w.recoveryInvocationUnchanged)
	sc.Step(`^a recovery fixture with usage variant "([^"]*)" and outside sentinels$`, func(variant string) error {
		if err := w.recoveryInvocationFixture(); err != nil {
			return err
		}
		id := strings.Repeat("a", 64)
		variants := map[string][]string{
			"stop missing ID": {"stop"}, "stop extra argument": {"stop", id, "extra"},
			"stop empty ID": {"stop", ""}, "stop uppercase hex ID": {"stop", strings.ToUpper(id)},
			"stop abbreviated ID": {"stop", id[:12]}, "stop intent path": {"stop", "intents/" + id + "/" + id + ".json"},
			"stop path traversal": {"stop", "../slice-11-outside"}, "stop shell control text": {"stop", "$(touch slice-11-marker);\n"},
			"status positional ID": {"status", id},
		}
		args, ok := variants[variant]
		if !ok {
			return fmt.Errorf("unknown recovery usage fixture %s", variant)
		}
		w.recoveryLocal.args = append([]string{args[0], "--json"}, args[1:]...)
		return nil
	})
	sc.Step(`^the recovery command is invoked in JSON mode with that literal argv$`, func() error {
		if err := w.recoveryInvocation(w.recoveryLocal.args...); err != nil {
			return err
		}
		return w.recoveryFinishWatch()
	})
	sc.Step(`^recovery returns one schema-1 failure with recovery.usage exit 2 before state or engine access$`, func() error {
		r := w.recoveryLocal.results[0]
		var report recoveryReport
		if json.Unmarshal([]byte(r.stdout), &report) != nil || r.exit != 2 || report.Schema != 1 || report.OK || len(report.Problems) != 1 || report.Problems[0].Rule != "recovery.usage" || report.Problems[0].Exit != 2 || report.Sessions == nil || len(report.Sessions) != 0 {
			return fmt.Errorf("invalid recovery invocation did not return the typed schema-1 usage refusal: %+v", r)
		}
		return w.recoveryInvocationUnchanged()
	})
	sc.Step(`^no raw invalid ID or synthetic marker appears in recovery output and no sentinel changes$`, func() error {
		r := w.recoveryLocal.results[0]
		for _, invalid := range w.recoveryLocal.args[2:] {
			if invalid != "" && strings.Contains(r.stdout+r.stderr, invalid) {
				return fmt.Errorf("recovery echoed untrusted invocation data")
			}
		}
		return w.recoveryInvocationUnchanged()
	})
}

func (w *world) recoveryInvocationFixture() error {
	if err := w.approvalCheckout(); err != nil {
		return err
	}
	if err := w.approvalSeed("1", false); err != nil {
		return err
	}
	if err := w.doctorSentinels(); err != nil {
		return err
	}
	w.recoveryLocal = &recoveryLocal{}
	var err error
	w.recoveryLocal.before, err = doctorTree(w.dir)
	if err != nil {
		return err
	}
	w.recoveryLocal.watch, err = w.sessionWatchInputs()
	if err != nil {
		return err
	}
	fd := int(w.recoveryLocal.watch.file.Fd())
	for _, path := range []string{w.doctor.env["XDG_STATE_HOME"], filepath.Dir(w.approval.record)} {
		id, err := sessionInotifyAdd(fd, path)
		if err != nil {
			return err
		}
		w.recoveryLocal.watch.paths[int32(id)] = path
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = w.recoveryLocal.watch.file.Close() })
	return nil
}

func (w *world) recoveryInvocation(args ...string) error {
	w.deadline = 15 * time.Second
	if err := w.runEnv(w.doctor.checkout, w.env(), w.bin, args...); err != nil {
		return err
	}
	w.recoveryLocal.results = append(w.recoveryLocal.results, doctorResult{exit: w.exit, stdout: w.stdout, stderr: w.stderr})
	return nil
}
func (w *world) recoveryFinishWatch() error {
	var err error
	w.recoveryLocal.reads, err = w.recoveryLocal.watch.finish()
	return err
}
func (w *world) recoveryHelp() error {
	if len(w.recoveryLocal.results) != 2 {
		return fmt.Errorf("missing help result")
	}
	for i, r := range w.recoveryLocal.results {
		if r.exit != 0 || r.stderr != "" {
			return fmt.Errorf("recovery help failed: %+v", r)
		}
		for _, text := range []string{"--json", "--no-json", "ITOS_JSON", "project", "0", "1", "2", "3", "70", "75"} {
			if !strings.Contains(r.stdout, text) {
				return fmt.Errorf("recovery help omits %s", text)
			}
		}
		if i == 0 && !strings.Contains(r.stdout, "no positional") {
			return fmt.Errorf("status help omits no positional arguments")
		}
		if i == 1 && !strings.Contains(r.stdout, "64") {
			return fmt.Errorf("stop help omits exact session ID")
		}
	}
	return nil
}
func (w *world) recoveryInvocationUnchanged() error {
	if len(w.recoveryLocal.reads) != 0 || w.doctor.engine.listener.connections.Load() != 0 {
		return fmt.Errorf("recovery invocation opened protected inputs or engine")
	}
	after, err := doctorTree(w.dir)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(w.recoveryLocal.before, after) {
		return fmt.Errorf("recovery invocation changed state or sentinels")
	}
	_, err = os.Stat(filepath.Join(w.doctor.checkout, "slice-11-marker"))
	if !os.IsNotExist(err) {
		return fmt.Errorf("literal shell text executed")
	}
	return w.doctorNoMarker()
}
