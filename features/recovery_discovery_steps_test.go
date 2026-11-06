package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) recoveryDiscoverySteps(sc *godog.ScenarioContext) {
	sc.Step(`^a recovery fixture with a running worker, a held controller lease and an unrelated live process$`, func() error {
		if err := w.recoverySeed("running", "running", "unknown"); err != nil {
			return err
		}
		lease, err := w.recoveryLock(filepath.Join(filepath.Dir(w.recoveryRecordPath()), "controller.lock"))
		if err != nil {
			return err
		}
		w.recoveryLocal.lease = lease
		path := filepath.Join(w.dir, "unrelated-lease")
		if err = os.WriteFile(path, nil, 0600); err != nil {
			return err
		}
		w.recoveryLocal.unrelated, err = w.recoveryLock(path)
		return err
	})
	sc.Step(`^status is called before and after the fixture lease holder exits$`, func() error {
		if err := w.recoveryInvocation("status", "--json"); err != nil {
			return err
		}
		if err := w.recoveryLocal.lease.resume(); err != nil {
			return err
		}
		w.recoveryLocal.lease.process.wait()
		return w.recoveryInvocation("status", "--json")
	})
	sc.Step(`^status changes controller from present to absent and stop candidate from false to true with the exact retry argv$`, func() error {
		for i, r := range w.recoveryLocal.results {
			report, err := recoveryLocalReport(r, 0)
			if err != nil {
				return err
			}
			if len(report.Sessions) != 1 {
				return fmt.Errorf("controller status lost the session")
			}
			e := report.Sessions[0]
			want := []string{"present", "absent"}[i]
			if e.Controller != want || e.StopCandidate != (i == 1) || !reflect.DeepEqual(e.Retry, []string{"itos", "container", "stop", w.recoveryLocal.id}) {
				return fmt.Errorf("lease observation wrong: %+v", e)
			}
		}
		return nil
	})
	sc.Step(`^the worker remains reported running with unknown workload exit$`, func() error {
		for _, r := range w.recoveryLocal.results {
			if _, err := w.recoveryLocalEntry(r, "running", "running", "pending", "unknown"); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^status neither signals the unrelated process nor changes resources or protected records$`, func() error {
		select {
		case <-w.recoveryLocal.unrelated.process.done:
			return fmt.Errorf("unrelated process was stopped")
		default:
		}
		return w.recoveryUnchanged()
	})
	sc.Step(`^a recovery fixture with safe sessions and separate invalid-state, unsafe-state and unavailable-engine entries$`, func() error {
		if err := w.recoverySeed("running", "running", "unknown"); err != nil {
			return err
		}
		for _, letter := range []string{"a", "c", "d", "e"} {
			if err := w.recoveryAdditional(letter); err != nil {
				return err
			}
		}
		root := filepath.Dir(filepath.Dir(w.recoveryRecordPath()))
		if err := os.WriteFile(filepath.Join(root, strings.Repeat("c", 64), "record.json"), []byte("{invalid"), 0600); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Join(root, strings.Repeat("d", 64), "record.json"), 0644); err != nil {
			return err
		}
		w.recoveryLocal.unavailableIDs = map[string]bool{approvalHash(map[string]any{"recoveryAdditional": "e"}): true}
		return w.recoveryCapture()
	})
	sc.Step(`^recovery status is invoked in JSON mode through each local backend$`, func() error { return w.recoveryInvocation("status", "--json") })
	sc.Step(`^status lists the independently safe sessions in ID order and reports recovery.invalid-state, recovery.unsafe-state and recovery.temporary with exit 75 and ok false$`, func() error {
		r, err := recoveryLocalReport(w.recoveryLocal.results[0], 75)
		if err != nil {
			return err
		}
		ids := []string{}
		for _, e := range r.Sessions {
			ids = append(ids, e.ID)
		}
		want := []string{strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("e", 64)}
		if !reflect.DeepEqual(ids, want) {
			return fmt.Errorf("partial status discarded or adopted entries: %v", ids)
		}
		keys := []string{}
		for _, p := range r.Problems {
			keys = append(keys, p.Session+"/"+p.Rule)
		}
		expected := []string{strings.Repeat("c", 64) + "/recovery.invalid-state", strings.Repeat("d", 64) + "/recovery.unsafe-state", strings.Repeat("e", 64) + "/recovery.temporary"}
		if !sort.StringsAreSorted(keys) || !reflect.DeepEqual(keys, expected) {
			return fmt.Errorf("partial status problems differ: %v", keys)
		}
		return nil
	})
	sc.Step(`^the unavailable session has unknown resource and cleanup without a fabricated exit or stopped claim$`, func() error {
		var r recoveryReport
		_ = json.Unmarshal([]byte(w.recoveryLocal.results[0].stdout), &r)
		for _, e := range r.Sessions {
			if e.ID == strings.Repeat("e", 64) {
				if e.Resource != "unknown" || e.Cleanup != "unknown" || e.State != "failed" || e.WorkloadExit != nil {
					return fmt.Errorf("partial status fabricated a fact")
				}
				return nil
			}
		}
		return fmt.Errorf("unavailable session omitted")
	})
	sc.Step(`^malformed entries confer no ownership and all recovery state remains unchanged$`, w.recoveryUnchanged)
}

func (w *world) recoveryAdditional(letter string) error {
	id := strings.Repeat(letter, 64)
	token := strings.Repeat(letter, 63) + "1"
	worker := approvalHash(map[string]any{"recoveryAdditional": letter})
	intent := w.sessionExpectedRecord("owned", worker)
	intent["name"] = "itos-container-" + token
	path := filepath.Join(filepath.Dir(w.sessionRecordPath()), token+".json")
	if err := os.WriteFile(path, approvalJSON(intent), 0600); err != nil {
		return err
	}
	public := map[string]any{"schema": 1, "id": id, "project": w.approval.project, "intent": "intents/" + w.approval.project + "/" + token + ".json", "state": "running", "workload_exit": nil, "last_problem": nil}
	dir := filepath.Join(filepath.Dir(filepath.Dir(w.recoveryRecordPath())), id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "record.json"), approvalJSON(public), 0600); err != nil {
		return err
	}
	for _, name := range []string{"controller.lock", "operation.lock"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			return err
		}
	}
	c := approvalClone(w.session.api.local[w.session.seedID])
	c["Id"], c["Name"] = worker, "/itos-container-"+token
	object(object(c["Config"])["Labels"])["io.itos-container.intent"] = token
	w.session.api.local[worker] = c
	return nil
}

func (w *world) recoveryLock(path string) (*recoveryProbe, error) {
	prefix := filepath.Join(w.dir, fmt.Sprintf("slice-11-lock-%d", len(w.recovery.processes)))
	if err := recoveryBuildHelper(w.root, prefix, recoveryLockSource); err != nil {
		return nil, err
	}
	p := &recoveryProbe{ready: prefix + ".ready", release: prefix + ".release"}
	original := w.bin
	w.bin = prefix
	p.process = w.recoveryStart(path, p.ready, p.release)
	w.bin = original
	if !p.await() {
		return nil, fmt.Errorf("fixture lock process failed")
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = p.resume(); p.process.wait() })
	return p, nil
}
