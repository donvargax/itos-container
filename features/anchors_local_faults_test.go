package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) anchorRecoveryFaultSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an anchor recovery fixture with worker uncertainty "([^"]*)" and a retained original owned volume$`, func(fault string) error { return w.anchorRecoveryFaultSeed("worker", fault) })
	sc.Step(`^status and stop inspect the uncertainty and a fresh stop retries after the fixture resolves it through both local backends$`, w.anchorRecoveryFaultAction)
	sc.Step(`^the first stop reports recovery.temporary with exit 75 and retained anchor present without any anchor delete or complete claim$`, func() error {
		return errors.Join(w.anchorRecoveryFaultResult("present"), w.anchorRecoveryMemoryResult())
	})
	sc.Step(`^the earlier status retains the original session anchor observation unknown workload exit and exact retry without mutating state$`, w.anchorRecoveryPriorStatus)
	sc.Step(`^the retry durably owns any reconciled full ID before removal and releases only the original anchor after confirmed worker absence$`, w.anchorRecoveryReconciled)
	sc.Step(`^neither attempt creates starts reallocates remounts or substitutes a checkout path and the retry completes durably$`, func() error {
		return errors.Join(w.anchorRecoveryStopResult(2, w.anchorLocalRecovery.exit), w.anchorRecoveryDurable(), w.anchorRecoveryPreserved())
	})
	sc.Step(`^an anchor recovery fixture with a recorded exit of 7 and persistent cleanup fault "([^"]*)"$`, func(fault string) error { return w.anchorRecoveryFaultSeed("cleanup", fault) })
	sc.Step(`^stop is attempted and a fresh stop retries after clearing the fault through both anchor backends$`, w.anchorRecoveryFaultAction)
	sc.Step(`^the first stop reaches confirmed worker absence and the anchor cleanup boundary before reporting recovery.temporary with exit 75$`, func() error {
		err := w.anchorRecoveryFaultResult(w.anchorLocalRecovery.firstVolume)
		if !w.anchorLocalRecovery.reached {
			err = errors.Join(err, fmt.Errorf("anchor cleanup fault was never reached"))
		}
		return errors.Join(err, w.anchorRecoveryMemoryResult())
	})
	sc.Step(`^its anchor fact matches the actual observation and it retains original ownership exit 7 and exact retry without successful completion$`, func() error { return w.anchorRecoveryFaultResult(w.anchorLocalRecovery.firstVolume) })
	sc.Step(`^the retry confirms both resources absent and establishes durable completion without recreating a volume or repeating confirmed removal$`, func() error {
		return errors.Join(w.anchorRecoveryStopResult(2, "7"), w.anchorRecoveryDurable(), w.anchorRecoveryNoRepeatRemoval())
	})
	sc.Step(`^the fault-free anchor control completes worker removal volume release and durable completion in that order$`, w.anchorRecoveryControlResult)
}

func (w *world) anchorRecoveryFaultSeed(mode, fault string) error {
	exit := "unknown"
	if mode == "cleanup" {
		exit = "7"
	}
	if err := w.anchorRecoverySeed("failed", "created", "present", "pending", exit); err != nil {
		return err
	}
	a := w.anchorLocalRecovery
	a.mode, a.fault = mode, fault
	if mode == "cleanup" {
		if err := w.anchorRecoveryControlSeed(); err != nil {
			return err
		}
		if strings.Contains(fault, "fsync") || strings.Contains(fault, "short write") {
			w.recoveryLocal.fault = fault
		}
		return nil
	}
	switch fault {
	case "create reply lost and exact-name lookup absent":
		a.savedWorker = w.session.api.local[w.session.seedID]
		delete(w.session.api.local, w.session.seedID)
		a.intent["stage"], a.intent["container_id"] = "intent", ""
		if err := os.WriteFile(w.sessionRecordPath(), approvalJSON(a.intent), 0600); err != nil {
			return err
		}
	case "worker removal fails with the full ID still present":
		w.recoveryLocal.failRemove = true
	case "worker removal and full-ID absence remain unknown":
		w.recoveryLocal.lostRemove = true
	default:
		return fmt.Errorf("unknown worker uncertainty %s", fault)
	}
	return w.recoveryCapture()
}

func (w *world) anchorRecoveryControlSeed() error {
	dir, err := os.MkdirTemp("", "slice-17-anchor-control-")
	if err != nil {
		return err
	}
	c := &world{root: w.root, bin: w.bin, dir: dir}
	w.anchorLocalRecovery.control = c
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = c.closeRecoveryNative(); _ = c.closeSession(); c.closeDoctor(); _ = os.RemoveAll(dir) })
	if err := c.anchorRecoverySeed("failed", "created", "present", "pending", "7"); err != nil {
		return err
	}
	old := filepath.Dir(c.recoveryRecordPath())
	c.recoveryLocal.id = strings.Repeat("e", 64)
	c.recovery.id = c.recoveryLocal.id
	if err := os.Rename(old, filepath.Dir(c.recoveryRecordPath())); err != nil {
		return err
	}
	data, err := os.ReadFile(c.recoveryRecordPath())
	if err != nil {
		return err
	}
	var p map[string]any
	if err = json.Unmarshal(data, &p); err != nil {
		return err
	}
	p["id"] = c.recoveryLocal.id
	if err = os.WriteFile(c.recoveryRecordPath(), approvalJSON(p), 0600); err != nil {
		return err
	}
	object(c.anchorLocalRecovery.intent["anchor"])["session"] = c.recoveryLocal.id
	if err = os.WriteFile(c.sessionRecordPath(), approvalJSON(c.anchorLocalRecovery.intent), 0600); err != nil {
		return err
	}
	object(c.session.api.volumes[c.anchorLocalRecovery.name]["Labels"])["io.itos-container.session"] = c.recoveryLocal.id
	return c.recoveryCapture()
}

func (w *world) anchorRecoveryFaultAction() error {
	if err := w.anchorRecoveryBeginWatch(); err != nil {
		return err
	}
	a := w.anchorLocalRecovery
	a.problem = errors.Join(a.problem, w.recoveryInvocation("status", "--json"))
	invoke := func() error {
		if w.recoveryLocal.fault != "" {
			return w.recoveryProbeInvoke("stop", w.recoveryLocal.id)
		}
		return w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id)
	}
	a.problem = errors.Join(a.problem, invoke())
	requests := w.sessionRequests()
	w.session.api.mu.Lock()
	a.firstRequests = len(requests)
	a.firstVolume = "absent"
	if w.session.api.volumes[a.name] != nil {
		a.firstVolume = "present"
	}
	if a.fault == "volume delete reply lost and inspection unavailable" {
		a.firstVolume = "unknown"
	}
	for _, q := range requests {
		if q.Method == "DELETE" && q.Path == "/volumes/"+a.name {
			a.reached = true
		}
	}
	if a.savedWorker != nil {
		w.session.api.local[w.session.seedID] = a.savedWorker
	}
	w.recoveryLocal.failRemove, w.recoveryLocal.lostRemove, w.recoveryLocal.unavailable = false, false, false
	w.recoveryLocal.fault = ""
	originalFault := a.fault
	a.fault = ""
	w.session.api.mu.Unlock()
	a.problem = errors.Join(a.problem, w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id))
	w.session.api.mu.Lock()
	a.fault = originalFault
	w.session.api.mu.Unlock()
	if a.control != nil {
		a.problem = errors.Join(a.problem, a.control.recoveryTracedInvocation("stop", "--json", a.control.recoveryLocal.id))
	}
	return w.anchorRecoveryMemory()
}

func (w *world) anchorRecoveryFaultResult(volume string) error {
	if len(w.recoveryLocal.results) < 2 {
		return fmt.Errorf("missing first stop")
	}
	if err := recoveryHasProblem(w.recoveryLocal.results[1], "recovery.temporary", 75); err != nil {
		return err
	}
	e, err := w.anchorRecoveryEntryAt(1, 75)
	if err != nil {
		return err
	}
	if e["anchor"] != volume || e["state"] == "stopped" || e["cleanup"] == "complete" {
		return fmt.Errorf("uncertainty lost original anchor fact: %v", e)
	}
	if w.anchorLocalRecovery.mode == "worker" {
		for _, q := range w.sessionRequests()[:w.anchorLocalRecovery.firstRequests] {
			if q.Method == "DELETE" && strings.HasPrefix(q.Path, "/volumes/") {
				return fmt.Errorf("uncertain worker allowed anchor release")
			}
		}
	}
	if fmt.Sprint(e["retry"]) != fmt.Sprint([]string{"itos", "container", "stop", w.recoveryLocal.id}) {
		return fmt.Errorf("uncertainty lost exact retry")
	}
	if w.anchorLocalRecovery.exit == "unknown" && e["workload_exit"] != nil {
		return fmt.Errorf("uncertainty fabricated exit")
	}
	if w.anchorLocalRecovery.exit == "7" && e["workload_exit"] != float64(7) {
		return fmt.Errorf("uncertainty lost exit 7")
	}
	return nil
}
func (w *world) anchorRecoveryPriorStatus() error {
	code := 0
	if w.anchorLocalRecovery.savedWorker != nil {
		code = 75
	}
	e, err := w.anchorRecoveryEntryAt(0, code)
	if err != nil {
		return err
	}
	if e["anchor"] != "present" || e["workload_exit"] != nil || e["id"] != w.recoveryLocal.id || e["retry"] == nil {
		return fmt.Errorf("status lost retained source: %v", e)
	}
	return nil
}
func (w *world) anchorRecoveryReconciled() error {
	for _, q := range w.sessionRequests() {
		if q.Method == "DELETE" && q.Path == "/containers/"+w.session.seedID {
			if q.Record.Bytes == "" {
				return fmt.Errorf("worker removal lacks protected evidence")
			}
			var r struct {
				ContainerID string `json:"container_id"`
			}
			if err := json.Unmarshal([]byte(q.Record.Bytes), &r); err != nil || r.ContainerID != w.session.seedID {
				return fmt.Errorf("worker removed without durable reconciled ID")
			}
		}
	}
	return w.anchorRecoveryReleaseOrder()
}
func (w *world) anchorRecoveryControlResult() error {
	c := w.anchorLocalRecovery.control
	if c == nil {
		return fmt.Errorf("missing anchor positive control")
	}
	return errors.Join(c.anchorRecoveryStopResult(0, "7"), c.anchorRecoveryReleaseOrder(), c.anchorRecoveryDurable())
}
func (w *world) anchorRecoveryNoRepeatRemoval() error {
	removed := false
	for _, q := range w.sessionRequests()[:w.anchorLocalRecovery.firstRequests] {
		if q.Method == "DELETE" && q.Path == "/containers/"+w.session.seedID && q.Status == 204 {
			removed = true
		}
	}
	if removed {
		for _, q := range w.sessionRequests()[w.anchorLocalRecovery.firstRequests:] {
			if q.Method == "DELETE" && q.Path == "/containers/"+w.session.seedID {
				return fmt.Errorf("retry repeated confirmed worker removal")
			}
		}
	}
	return nil
}
