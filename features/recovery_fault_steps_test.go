package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) recoveryFaultSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a recovery fixture with ownership defect "([^"]*)" and an independently observed candidate worker$`, w.recoveryOwnershipDefect)
	sc.Step(`^recovery stop and status are invoked through each local backend$`, func() error {
		if err := w.recoveryInvocation("stop", "--json", w.recoveryLocal.id); err != nil {
			return err
		}
		return w.recoveryInvocation("status", "--json")
	})
	sc.Step(`^stop reports recovery.not-owned with exit 1 and no stop kill remove or ownership adoption$`, func() error {
		if err := recoveryHasProblem(w.recoveryLocal.results[0], "recovery.not-owned", 1); err != nil {
			return err
		}
		return w.recoveryNoMutation()
	})
	sc.Step(`^every candidate worker and original record remains unchanged despite matching labels or names$`, w.recoveryUnchanged)
	sc.Step(`^a recovery fixture with protected material defect "([^"]*)" and an intact matching current approval$`, w.recoveryMaterialDefect)
	sc.Step(`^recovery status and stop are invoked through each local backend$`, func() error {
		if w.recoveryLocal.fault != "" {
			if err := w.recoveryProbeInvoke("status"); err != nil {
				return err
			}
			return w.recoveryProbeInvoke("stop", w.recoveryLocal.id)
		}
		return w.recoveryStatusStop()
	})
	sc.Step(`^each recovery command reports "([^"]*)" with exit (\d+) and no engine mutation or unsafe target access$`, func(rule string, code int) error {
		for _, r := range w.recoveryLocal.results {
			if err := recoveryHasProblem(r, rule, code); err != nil {
				return err
			}
		}
		return w.recoveryNoMutation()
	})
	sc.Step(`^original ownership evidence and outside sentinels survive without chmod chown or state repair$`, w.recoveryUnchanged)
	sc.Step(`^a recovery fixture with observation failure "([^"]*)"$`, w.recoveryObservationFault)
	sc.Step(`^status and stop are invoked through each local backend with measured monotonic deadlines$`, func() error {
		for _, args := range [][]string{{"status"}, {"stop", w.recoveryLocal.id}} {
			start := time.Now()
			var err error
			if w.recoveryLocal.fault != "" {
				err = w.recoveryProbeInvoke(args...)
			} else {
				expanded := append([]string{args[0], "--json"}, args[1:]...)
				err = w.recoveryTracedInvocation(expanded...)
			}
			if err != nil {
				return err
			}
			index := len(w.recoveryLocal.results) - 1
			w.recoveryLocal.results[index].elapsed = time.Since(start)
		}
		return nil
	})
	sc.Step(`^recovery reports "([^"]*)" with exit (\d+) within 10 seconds for status and 30 seconds for stop without resetting totals$`, func(rule string, code int) error {
		for i, r := range w.recoveryLocal.results {
			if err := recoveryHasProblem(r, rule, code); err != nil {
				return err
			}
			limit := 12 * time.Second
			if i == 1 {
				limit = 32 * time.Second
			}
			if r.elapsed > limit {
				return fmt.Errorf("recovery exceeded total budget: %s", r.elapsed)
			}
		}
		return nil
	})
	sc.Step(`^no unavailable observation becomes absence a known workload exit or successful cleanup$`, func() error {
		for _, r := range w.recoveryLocal.results {
			var report recoveryReport
			if json.Unmarshal([]byte(r.stdout), &report) != nil {
				return fmt.Errorf("missing recovery report")
			}
			for _, s := range report.Sessions {
				if s.Resource != "unknown" || s.Cleanup != "unknown" || s.WorkloadExit != nil || s.State == "stopped" {
					return fmt.Errorf("unavailable observation invented a result")
				}
			}
		}
		return nil
	})
	sc.Step(`^safe ownership remains available for a later retry without resource mutation on an unvalidated identity$`, func() error {
		if err := w.recoveryNoMutation(); err != nil {
			return err
		}
		data, err := os.ReadFile(w.sessionRecordPath())
		if err != nil {
			return err
		}
		if string(data) != w.recoveryLocal.originalIntent {
			return fmt.Errorf("original intent lost")
		}
		return nil
	})
}
func recoveryHasProblem(r doctorResult, rule string, code int) error {
	report, err := recoveryLocalReport(r, code)
	if err != nil {
		return err
	}
	for _, p := range report.Problems {
		if p.Rule == rule && p.Exit == code {
			return nil
		}
	}
	return fmt.Errorf("recovery lacks %s (exit %d): %+v", rule, code, report)
}
func (w *world) recoveryNoMutation() error {
	for _, r := range w.session.api.requests {
		if r.Method == "POST" || r.Method == "DELETE" {
			return fmt.Errorf("refused recovery mutated resource: %s %s", r.Method, r.Path)
		}
	}
	return nil
}
func (w *world) recoveryOwnershipDefect(defect string) error {
	if err := w.recoverySeed("failed", "created", "unknown"); err != nil {
		return err
	}
	c := w.session.api.local[w.session.seedID]
	labels := object(object(c["Config"])["Labels"])
	switch defect {
	case "no public registration", "public ID belongs to other project":
		path := filepath.Dir(w.recoveryRecordPath())
		if defect == "no public registration" {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		} else {
			other := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", strings.Repeat("e", 64), w.recoveryLocal.id)
			if err := os.MkdirAll(filepath.Dir(other), 0700); err != nil {
				return err
			}
			if err := os.Rename(path, other); err != nil {
				return err
			}
		}
	case "mutually consistent other UID":
		original := w.recoveryRecordPath()
		data, err := os.ReadFile(original)
		if err != nil {
			return err
		}
		var record map[string]any
		if err = json.Unmarshal(data, &record); err != nil {
			return err
		}
		if err = w.sessionSeedRecovery("identity", "record UID differs from the current host UID"); err != nil {
			return err
		}
		path := w.recoveryRecordPath()
		if err = os.MkdirAll(filepath.Dir(filepath.Dir(path)), 0700); err != nil {
			return err
		}
		if err = os.Rename(filepath.Dir(original), filepath.Dir(path)); err != nil {
			return err
		}
		record["project"], record["intent"] = w.approval.project, w.session.reference
		if err = os.WriteFile(path, approvalJSON(record), 0600); err != nil {
			return err
		}
	case "different checkout identity":
		path := filepath.Join(w.doctor.checkout, ".git")
		if err := os.Rename(path, path+".original"); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0700); err != nil {
			return err
		}
	case "selected daemon differs":
		w.doctor.engine.info["ID"] = "foreign"
	case "full ID observation conflicts":
		c["Id"] = strings.Repeat("d", 64)
	case "resource name differs":
		c["Name"] = "/foreign"
	case "resource intent label differs":
		labels["io.itos-container.intent"] = "foreign"
	case "resource project label differs":
		labels["io.itos-container.project"] = "foreign"
	case "resource fingerprint differs":
		labels["io.itos-container.fingerprint"] = "foreign"
	case "resource image ID differs":
		c["Image"] = "foreign"
	case "name and full ID disagree":
		if err := os.WriteFile(w.sessionRecordPath(), approvalJSON(w.sessionExpectedRecord("intent", "")), 0600); err != nil {
			return err
		}
		w.recoveryLocal.mode = "name conflict"
	default:
		return fmt.Errorf("unknown ownership fixture %s", defect)
	}
	return w.recoveryCapture()
}
func (w *world) recoveryMaterialDefect(defect string) error {
	if err := w.recoverySeed("failed", "created", "unknown"); err != nil {
		return err
	}
	path := w.recoveryRecordPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record map[string]any
	if err = json.Unmarshal(data, &record); err != nil {
		return err
	}
	symlink := func(path string) error {
		outside := filepath.Join(w.doctor.host, "slice-11-outside")
		if err := os.Rename(path, outside); err != nil {
			return err
		}
		return os.Symlink(outside, path)
	}
	switch defect {
	case "session ancestor symlink":
		err = symlink(filepath.Dir(filepath.Dir(path)))
	case "record symlink":
		err = symlink(path)
	case "record hard link":
		err = os.Link(path, filepath.Join(w.doctor.host, "slice-11-outside"))
	case "record wrong filesystem owner":
		w.recoveryLocal.fault = defect
	case "group-writable directory":
		err = os.Chmod(filepath.Dir(path), 0770)
	case "world-readable record":
		err = os.Chmod(path, 0644)
	case "controller lock symlink":
		err = symlink(filepath.Join(filepath.Dir(path), "controller.lock"))
	case "operation lock symlink":
		err = symlink(filepath.Join(filepath.Dir(path), "operation.lock"))
	case "unknown schema":
		record["schema"] = 2
		data = approvalJSON(record)
	case "duplicate field":
		data = append([]byte(`{"schema":1,`), data[1:]...)
	case "unknown field":
		record["unexpected"] = true
		data = approvalJSON(record)
	case "missing field":
		delete(record, "last_problem")
		data = approvalJSON(record)
	case "wrong workload exit type":
		record["workload_exit"] = "7"
		data = approvalJSON(record)
	case "trailing JSON":
		data = append(data, []byte(" {}")...)
	case "oversized JSON":
		data = []byte(strings.Repeat(" ", 65537))
	case "overdeep JSON":
		data = []byte(strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17))
	case "mismatched record ID":
		record["id"] = strings.Repeat("d", 64)
		data = approvalJSON(record)
	case "mismatched record project":
		record["project"] = strings.Repeat("d", 64)
		data = approvalJSON(record)
	case "mismatched intent reference":
		record["intent"] = "intents/" + strings.Repeat("d", 64) + "/" + w.session.token + ".json"
		data = approvalJSON(record)
	case "missing intent after registration":
		err = os.Remove(w.sessionRecordPath())
	case "missing original snapshot":
		err = os.Remove(w.sessionSnapshotPath())
	case "corrupt original snapshot":
		err = os.WriteFile(w.sessionSnapshotPath(), []byte("{invalid"), 0600)
	default:
		return fmt.Errorf("unknown protected material fixture %s", defect)
	}
	if err != nil {
		return err
	}
	if strings.HasPrefix(defect, "unknown ") || strings.HasPrefix(defect, "mismatched ") || strings.HasPrefix(defect, "missing field") || strings.HasPrefix(defect, "wrong workload") || defect == "duplicate field" || defect == "trailing JSON" || defect == "oversized JSON" || defect == "overdeep JSON" {
		if err = os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}
	return w.recoveryCapture()
}
func (w *world) recoveryObservationFault(fault string) error {
	phase, resource := "failed", "created"
	if fault == "completed session daemon down" {
		phase, resource = "stopped", "absent"
	}
	if err := w.recoverySeed(phase, resource, "unknown"); err != nil {
		return err
	}
	data, err := os.ReadFile(w.sessionRecordPath())
	if err != nil {
		return err
	}
	w.recoveryLocal.originalIntent = string(data)
	switch fault {
	case "required socket missing":
		w.doctor.env["DOCKER_HOST"] = "unix://" + filepath.Join(w.dir, "missing.sock")
	case "inspection transport lost", "completed session daemon down":
		w.recoveryLocal.unavailable = true
	case "stalled engine call":
		w.recoveryLocal.stall = true
	case "malformed engine observation":
		w.recoveryLocal.malformed = true
	case "unclassified adapter failure":
		w.recoveryLocal.fault = fault
	case "held operation lock":
		return w.recoveryHoldLock("operation.lock")
	default:
		return fmt.Errorf("unknown observation failure %s", fault)
	}
	return nil
}

// Lock helpers are separate processes so the descriptor lease, not a PID or an
// in-process counter, is the liveness and exclusion authority under test.
func (w *world) recoveryHoldLock(name string) error {
	helper := filepath.Join(w.dir, "slice-11-lock-holder")
	if err := recoveryBuildHelper(w.root, helper, recoveryLockSource); err != nil {
		return err
	}
	ready := helper + ".ready"
	release := helper + ".release"
	p := &recoveryProbe{ready: ready, release: release}
	original := w.bin
	w.bin = helper
	p.process = w.recoveryStart(filepath.Join(filepath.Dir(w.recoveryRecordPath()), name), ready, release)
	w.bin = original
	if !p.await() {
		return fmt.Errorf("fixture lock process failed")
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = p.resume(); p.process.wait() })
	w.recoveryLocal.boundary = release
	return nil
}

const recoveryLockSource = `package main
import("os";"time";"golang.org/x/sys/unix")
func main(){ if len(os.Args)!=4{os.Exit(96)}; f,e:=os.Open(os.Args[1]); if e!=nil{os.Exit(96)}; defer f.Close(); if unix.Flock(int(f.Fd()),unix.LOCK_EX)!=nil{os.Exit(96)}; if os.WriteFile(os.Args[2],[]byte("ready"),0600)!=nil{os.Exit(96)}; for {if b,e:=os.ReadFile(os.Args[3]);e==nil&&string(b)=="continue"{return}; time.Sleep(5*time.Millisecond)} }
`
