package features

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) anchorRecoveryRaceSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an anchor recovery fixture with a final-release barrier and substitution "([^"]*)"$`, func(change string) error {
		if err := w.anchorRecoverySeed("failed", "created", "present", "pending", "unknown"); err != nil {
			return err
		}
		a := w.anchorLocalRecovery
		a.mode, a.fault = "barrier", change
		a.ready, a.release = make(chan struct{}), make(chan struct{})
		return w.anchorRecoveryControlSeed()
	})
	sc.Step(`^stop reaches the anchor release barrier and the fixture substitutes authority before releasing it through both local backends$`, w.anchorRecoveryRace)
	sc.Step(`^stop reaches that boundary after worker absence and returns recovery.not-owned with exit 1 before the barred volume delete$`, func() error {
		var err error
		if !w.anchorLocalRecovery.reached {
			err = fmt.Errorf("anchor release boundary not reached after worker absence")
		}
		if len(w.recoveryLocal.results) != 0 {
			err = errors.Join(err, recoveryHasProblem(w.recoveryLocal.results[0], "recovery.not-owned", 1))
		}
		for _, q := range w.sessionRequests() {
			if q.Method == "DELETE" && strings.HasPrefix(q.Path, "/volumes/") {
				err = errors.Join(err, fmt.Errorf("substitution permitted anchor deletion"))
			}
		}
		return errors.Join(err, w.anchorRecoveryMemoryResult())
	})
	sc.Step(`^no replacement target is changed and original incomplete anchor recovery stays discoverable with its exact retry$`, func() error {
		if err := w.anchorRecoveryVolumesUnchanged(); err != nil {
			return err
		}
		data, err := os.ReadFile(w.recoveryRecordPath())
		if err != nil {
			return err
		}
		if strings.Contains(string(data), `"stopped"`) {
			return fmt.Errorf("substitution completed cleanup")
		}
		if len(w.recoveryLocal.results) == 0 || !strings.Contains(w.recoveryLocal.results[0].stdout, w.recoveryLocal.id) {
			return fmt.Errorf("lost recovery identity")
		}
		return nil
	})
	sc.Step(`^the matching no-substitution anchor control releases its owned volume and completes durably$`, w.anchorRecoveryControlResult)
	sc.Step(`^disjoint equal-basename anchor recovery fixtures A and B with an A volume-release barrier$`, func() error {
		if err := w.anchorRecoverySeed("failed", "created", "present", "pending", "unknown"); err != nil {
			return err
		}
		a := w.anchorLocalRecovery
		a.mode = "concurrent"
		a.ready, a.release = make(chan struct{}), make(chan struct{})
		return w.anchorRecoveryControlSeed()
	})
	sc.Step(`^two fresh A stops overlap at that barrier while B is inspected and stopped before A is released through both local backends$`, w.anchorRecoveryConcurrent)
	sc.Step(`^B completes its own worker and anchor cleanup while A is paused and both A stops later complete through one owned anchor deletion$`, func() error {
		var err error
		if !w.anchorLocalRecovery.reached {
			err = fmt.Errorf("A anchor barrier not reached")
		}
		return errors.Join(err, w.anchorRecoveryStopResult(0, "unknown"), w.anchorRecoveryStopResult(1, "unknown"), w.anchorRecoveryControlResult(), w.anchorRecoveryMemoryResult())
	})
	sc.Step(`^the waiting A stop reloads completion without repeated termination removal volume deletion or completion rewrite$`, func() error {
		count := 0
		for _, q := range w.sessionRequests() {
			if q.Method == "DELETE" && q.Path == "/volumes/"+w.anchorLocalRecovery.name {
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf("concurrent stops removed anchor %d times", count)
		}
		return w.anchorRecoveryDurable()
	})
	sc.Step(`^concurrent A status observes a consistent worker and anchor state or bounded recovery.temporary without state writes$`, func() error {
		if len(w.recoveryLocal.results) < 3 {
			return fmt.Errorf("missing concurrent status")
		}
		r := w.recoveryLocal.results[2]
		if r.exit == 75 {
			return recoveryHasProblem(r, "recovery.temporary", 75)
		}
		return w.anchorRecoveryStopResult(2, "unknown")
	})
	sc.Step(`^stopping B's ID from A refuses ownership without contacting B and all unrelated resources and checkout edits survive$`, func() error {
		if len(w.recoveryLocal.results) < 4 {
			return fmt.Errorf("missing cross-project stop")
		}
		return errors.Join(recoveryHasProblem(w.recoveryLocal.results[3], "recovery.not-owned", 1), w.anchorRecoveryPreserved())
	})
	sc.Step(`^an anchor recovery fixture configured to stall "([^"]*)" after prior delays consume most of the invocation budget$`, func(operation string) error {
		if err := w.anchorRecoverySeed("failed", "created", "present", "pending", "7"); err != nil {
			return err
		}
		a := w.anchorLocalRecovery
		a.mode, a.fault = "budget", operation
		return nil
	})
	sc.Step(`^the anchor recovery command runs and a fresh stop retries after the stall is cleared through both local backends$`, w.anchorRecoveryBudget)
	sc.Step(`^the named anchor operation is reached and reports recovery.temporary with exit 75 within the original (\d+) second total$`, func(seconds int) error {
		a := w.anchorLocalRecovery
		var err error
		if !a.reached {
			err = fmt.Errorf("named anchor budget operation not reached")
		}
		if len(w.recoveryLocal.results) == 0 {
			return errors.Join(err, fmt.Errorf("missing budget result"))
		}
		r := w.recoveryLocal.results[0]
		if r.elapsed > time.Duration(seconds+2)*time.Second {
			err = errors.Join(err, fmt.Errorf("anchor reset total: %s", r.elapsed))
		}
		return errors.Join(err, recoveryHasProblem(r, "recovery.temporary", 75), w.anchorRecoveryMemoryResult())
	})
	sc.Step(`^no expiry becomes anchor absence successful cleanup or authority for another destructive call or progress write$`, func() error {
		e, err := w.anchorRecoveryEntryAt(0, 75)
		if err != nil {
			return err
		}
		if e["cleanup"] == "complete" || e["anchor"] == "absent" {
			return fmt.Errorf("deadline invented absence")
		}
		return nil
	})
	sc.Step(`^original ownership and workload knowledge survive and the fresh retry completes both resource absences durably$`, func() error { return errors.Join(w.anchorRecoveryStopResult(1, "7"), w.anchorRecoveryDurable()) })
}

func (a *sessionAPI) anchorRecoveryBarrier(request *http.Request, q sessionRequest) {
	s := a.w.anchorLocalRecovery
	if s == nil || s.held || s.mode != "barrier" && s.mode != "concurrent" || a.local[a.w.session.seedID] != nil || q.Path != "/volumes/"+s.name {
		return
	}
	s.held, s.reached = true, true
	close(s.ready)
	a.mu.Unlock()
	select {
	case <-s.release:
	case <-request.Context().Done():
	}
	a.mu.Lock()
}

func (w *world) anchorRecoveryRace() error {
	if err := w.anchorRecoveryBeginWatch(); err != nil {
		return err
	}
	a := w.anchorLocalRecovery
	p := w.recoveryStart("stop", "--json", w.recoveryLocal.id)
	if recoveryBarrier(a.ready, p) {
		w.session.api.mu.Lock()
		switch a.fault {
		case "pinned socket inode replaced":
			path := strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://")
			a.problem = os.Rename(path, path+".original")
			if a.problem == nil {
				a.problem = os.WriteFile(path, []byte("foreign socket target"), 0600)
			}
		case "selected daemon changed":
			w.doctor.engine.info["ID"] = "changed-daemon"
		case "same-named volume tuple changed":
			w.session.api.volumes[a.name]["CreatedAt"] = "2026-11-07T18:00:00Z"
		case "protected anchor record replaced":
			data, err := os.ReadFile(w.sessionRecordPath())
			a.problem = err
			if err == nil {
				data = []byte(strings.Replace(string(data), `/proc/2147483647/fd/17`, `/proc/2147483646/fd/19`, 1))
				temp := w.sessionRecordPath() + ".replacement"
				a.problem = os.WriteFile(temp, data, 0600)
				if a.problem == nil {
					a.problem = os.Rename(temp, w.sessionRecordPath())
				}
			}
		}
		a.volumes = map[string]map[string]any{}
		for name, v := range w.session.api.volumes {
			a.volumes[name] = approvalClone(v)
		}
		w.session.api.mu.Unlock()
	}
	close(a.release)
	a.problem = errors.Join(a.problem, w.recoveryCollect(p), a.control.recoveryTracedInvocation("stop", "--json", a.control.recoveryLocal.id))
	return w.anchorRecoveryMemory()
}

func (w *world) anchorRecoveryConcurrent() error {
	if err := w.anchorRecoveryBeginWatch(); err != nil {
		return err
	}
	a := w.anchorLocalRecovery
	first := w.recoveryStart("stop", "--json", w.recoveryLocal.id)
	if !recoveryBarrier(a.ready, first) {
		close(a.release)
		a.problem = w.recoveryCollect(first)
		return w.anchorRecoveryMemory()
	}
	second := w.recoveryStart("stop", "--json", w.recoveryLocal.id)
	inspection := w.recoveryStart("status", "--json")
	c := a.control
	if err := c.anchorRecoveryBeginWatch(); err != nil {
		close(a.release)
		return err
	}
	a.problem = c.recoveryInvocation("status", "--json")
	if len(c.recoveryLocal.results) == 1 {
		a.controlStatus = c.recoveryLocal.results[0]
		c.recoveryLocal.results = nil
	}
	a.problem = errors.Join(a.problem, c.recoveryTracedInvocation("stop", "--json", c.recoveryLocal.id))
	if a.controlStatus.exit != 0 || !strings.Contains(a.controlStatus.stdout, `"anchor":"present"`) {
		a.problem = errors.Join(a.problem, fmt.Errorf("B status lost its retained anchor: %+v", a.controlStatus))
	}
	if c.session.api.local[c.session.seedID] != nil || c.session.api.volumes[c.anchorLocalRecovery.name] != nil {
		a.problem = errors.Join(a.problem, fmt.Errorf("B did not complete while A paused"))
	}
	close(a.release)
	a.problem = errors.Join(a.problem, w.recoveryCollect(first), w.recoveryCollect(second), w.recoveryCollect(inspection))
	// IDs are independently seeded for the two projects. A foreign exact ID
	// must not acquire authority just because its basename matches.
	foreign := c.recoveryLocal.id
	beforeForeign := len(c.sessionRequests())
	a.problem = errors.Join(a.problem, w.recoveryInvocation("stop", "--json", foreign))
	if len(c.sessionRequests()) != beforeForeign {
		a.problem = errors.Join(a.problem, fmt.Errorf("cross-project stop contacted B"))
	}
	return w.anchorRecoveryMemory()
}

func (w *world) anchorRecoveryBudget() error {
	if err := w.anchorRecoveryBeginWatch(); err != nil {
		return err
	}
	a := w.anchorLocalRecovery
	command := "stop"
	if a.fault == "status anchor inspect" {
		command = "status"
	}
	if command == "stop" {
		if err := w.recoveryHoldLock("operation.lock"); err != nil {
			return err
		}
		release := w.recoveryLocal.boundary
		go func() { time.Sleep(25 * time.Second); _ = os.WriteFile(release, []byte("continue"), 0600) }()
	}
	start := time.Now()
	args := []string{command, "--json"}
	if command == "stop" {
		args = append(args, w.recoveryLocal.id)
	}
	a.problem = w.recoveryTracedInvocation(args...)
	if len(w.recoveryLocal.results) > 0 {
		w.recoveryLocal.results[0].elapsed = time.Since(start)
	}
	fault := a.fault
	a.fault = ""
	a.problem = errors.Join(a.problem, w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id))
	a.fault = fault
	return w.anchorRecoveryMemory()
}
