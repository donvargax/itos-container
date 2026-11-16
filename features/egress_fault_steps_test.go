package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

type egressLifeFault struct {
	name, storage, receipt string
	before, afterRetry     map[string]string
	log                    []byte
	retained               map[string]string
}

func (w *world) egressFaultSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a local egress lifecycle fixture with an observed zero workload exit and fault "([^"]*)"$`, w.egressFaultFixture)
	sc.Step(`^run tears down and a separate status inspects the retained session$`, w.egressFaultRun)
	sc.Step(`^run reports "([^"]*)" with exit (\d+) and the original public session's exact stop retry$`, w.egressFaultResult)
	sc.Step(`^status reports refusal evidence "([^"]*)" separately from incomplete cleanup$`, w.egressFaultStatus)
	sc.Step(`^the stopped proxy and every not-yet-released dependency retain their original owned recovery identity$`, w.egressFaultRetained)
	sc.Step(`^no partial destination is reported as a committed clean log and no later resource is removed past the fault$`, w.egressFaultNoPartial)
	sc.Step(`^the fault is removed and a fresh stop process retries twice$`, w.egressFaultRetry)
	sc.Step(`^the original log is committed without duplicate records and owned cleanup completes$`, w.egressFaultComplete)
	sc.Step(`^the retry reports the same sequence gaps if any without treating them as cleanup work$`, w.egressFaultGap)
	sc.Step(`^checkout edits approval snapshots and unrelated resources remain unchanged$`, w.egressFaultPreserved)
}

func (w *world) egressFaultFixture(name string) error {
	storage := map[string]string{
		"refusal copy cannot begin": "begin", "refusal copy write fails": "write",
		"refusal copy file fsync fails": "file fsync", "refusal copy rename fails": "rename",
		"refusal copy parent fsync fails": "parent fsync",
	}[name]
	if storage == "" && name != "proxy log transport fails mid-copy" && name != "gapped copy then proxy remove fails" {
		return fmt.Errorf("unknown copy fault %q", name)
	}
	if err := w.egressGapFixture("run", "0"); err != nil {
		return err
	}
	f := &egressLifeFault{name: name, storage: storage, receipt: filepath.Join(w.dir, "slice-28-storage-receipt")}
	w.egress.lifeFault = f
	sequences := "2 3 4"
	if name == "gapped copy then proxy remove fails" {
		sequences = "2 5 7"
	}
	if err := w.egressGapSource("1", sequences); err != nil {
		return err
	}
	if storage != "" {
		w.doctor.env["SLICE_28_STORAGE_FAULT"] = storage
		w.doctor.env["SLICE_28_STORAGE_RECEIPT"] = f.receipt
	}
	a := w.doctor.engine.egress
	a.mu.Lock()
	a.copyFault = name == "proxy log transport fails mid-copy"
	if name == "gapped copy then proxy remove fails" {
		a.removeFault = "proxy"
	}
	// Independent foreign objects are never candidates for owned cleanup.
	a.containers[strings.Repeat("e", 64)] = map[string]any{"Id": strings.Repeat("e", 64), "Name": "/slice-28-foreign", "State": map[string]any{"Running": true}}
	a.networks[strings.Repeat("f", 64)] = map[string]any{"Id": strings.Repeat("f", 64), "Name": "slice-28-foreign"}
	a.mu.Unlock()
	if err := os.WriteFile(filepath.Join(w.doctor.checkout, "slice-28-edit"), []byte("retained independently seeded edit"), 0600); err != nil {
		return err
	}
	var err error
	f.before, err = w.egressFaultProtectedInputs()
	return err
}

func (w *world) egressFaultProtectedInputs() (map[string]string, error) {
	result := map[string]string{}
	for _, root := range []string{w.doctor.checkout, w.egressStatePath("approvals"), w.egressStatePath("snapshots")} {
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			continue // a run's first snapshot does not yet exist
		}
		tree, err := doctorTree(root)
		if err != nil {
			return nil, err
		}
		for path, value := range tree {
			result[filepath.Join(root, path)] = value
		}
	}
	return result, nil
}

func (w *world) egressFaultRun() error {
	if err := w.egressInvoke([]string{"run", "--", "fixture-workload"}); err != nil {
		return err
	}
	delete(w.doctor.env, "SLICE_28_STORAGE_FAULT")
	delete(w.doctor.env, "SLICE_28_STORAGE_RECEIPT")
	entries, _ := os.ReadDir(w.egressStatePath("sessions", w.recoveryIndexKey()))
	if len(entries) == 1 {
		w.egress.life.id = entries[0].Name()
	}
	var err error
	w.egress.lifeFault.retained, err = doctorTree(w.egressStatePath())
	if err != nil {
		return err
	}
	return w.egressInvoke([]string{"status", "--json"})
}

func (w *world) egressFaultResult(diagnoses string, exit int) error {
	f, l := w.egress.lifeFault, w.egress.life
	if f.storage != "" {
		data, err := os.ReadFile(f.receipt)
		if err != nil || string(data) != f.storage {
			return errors.New("run did not reach the named real storage operation")
		}
	} else {
		a := w.doctor.engine.egress
		a.mu.Lock()
		want := "copy transport"
		if f.name == "gapped copy then proxy remove fails" {
			want = "remove proxy"
		}
		reached := slices.Contains(a.faults, want)
		a.mu.Unlock()
		if !reached {
			return errors.New("run did not reach the named Engine API fault")
		}
	}
	r := w.egress.results[0].result
	if r.exit != exit || l.id == "" {
		return fmt.Errorf("run returned %d, want %d with its original public ID\n%s%s", r.exit, exit, r.stdout, r.stderr)
	}
	for _, rule := range strings.Split(diagnoses, ",") {
		if !strings.Contains(r.stderr, rule) {
			return fmt.Errorf("run omitted %s\n%s", rule, r.stderr)
		}
	}
	if !strings.Contains(r.stderr, "itos container stop "+l.id) {
		return errors.New("run omitted the exact public stop retry")
	}
	return nil
}

func egressLifeEntry(r egressResult, id string) (map[string]any, []map[string]any, error) {
	var report struct {
		Sessions []map[string]any
		Problems []map[string]any
	}
	if err := json.Unmarshal([]byte(r.result.stdout), &report); err != nil {
		return nil, nil, fmt.Errorf("%s has no JSON report", r.command)
	}
	for _, entry := range report.Sessions {
		if entry["id"] == id {
			return entry, report.Problems, nil
		}
	}
	return nil, report.Problems, errors.New("report lost the original session")
}

func (w *world) egressFaultStatus(state string) error {
	e, _, err := egressLifeEntry(w.egress.results[1], w.egress.life.id)
	if err != nil {
		return err
	}
	report := object(object(e["egress"])["refusal_log"])
	if report["state"] != state || report["path"] != w.egressGapPath() {
		return fmt.Errorf("status has no %s refusal report for the retained session", state)
	}
	if (state == "pending" || state == "unknown") && report["records"] != nil {
		return errors.New("uncommitted evidence was reported with a stored count")
	}
	if state == "gapped" && (report["records"] != float64(3) || len(anyList(report["missing"])) != 2) {
		return errors.New("committed gap evidence lost its count or ranges")
	}
	if e["cleanup"] == "complete" || !sameJSON(e["retry"], []string{"itos", "container", "stop", w.egress.life.id}) {
		return errors.New("status lost pending cleanup or its exact retry")
	}
	return nil
}

func (w *world) egressFaultRetained() error {
	a := w.doctor.engine.egress
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return a.problem
	}
	proxy := a.containers[a.ids["proxy"]]
	if proxy == nil || object(proxy["State"])["Running"] != false || a.containers[a.ids["worker"]] != nil {
		return errors.New("copy failure lost the stopped proxy or kept a worker")
	}
	i, err := a.intent(a.token())
	if err != nil {
		return err
	}
	for _, role := range []string{"proxy", "egress", "internal"} {
		stage, id := egressStage(i, role)
		if stage != "owned" || id != a.ids[role] {
			return fmt.Errorf("retained %s lost original durable ownership", role)
		}
		if role != "proxy" && a.networks[id] == nil {
			return fmt.Errorf("retained %s was removed", role)
		}
	}
	anchor := object(i["anchor"])
	if anchor["stage"] != "owned" || a.volumes[fmt.Sprint(anchor["name"])] == nil {
		return errors.New("retained anchor lost original ownership")
	}
	return nil
}

func (w *world) egressFaultNoPartial() error {
	f := w.egress.lifeFault
	data, err := os.ReadFile(w.egressGapPath())
	if f.storage == "parent fsync" || f.name == "gapped copy then proxy remove fails" {
		if err != nil || string(data) != w.egress.life.stored {
			return errors.New("post-rename copy does not preserve the available original bytes")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("a failed unpublished copy exposed a final destination")
	}
	a := w.doctor.engine.egress
	a.mu.Lock()
	defer a.mu.Unlock()
	if !slices.Equal(a.removed, []string{"worker"}) {
		return errors.New("removal continued past failed log preservation or proxy removal")
	}
	return nil
}

func (w *world) egressFaultRetry() error {
	a := w.doctor.engine.egress
	a.mu.Lock()
	a.copyFault, a.removeFault = false, ""
	a.mu.Unlock()
	if err := w.egressInvoke([]string{"stop", w.egress.life.id, "--json"}, []string{"status", "--json"}); err != nil {
		return err
	}
	f := w.egress.lifeFault
	var err error
	f.afterRetry, err = doctorTree(w.egressStatePath())
	if err != nil {
		return err
	}
	f.log, _ = os.ReadFile(w.egressGapPath())
	return w.egressInvoke([]string{"stop", w.egress.life.id, "--json"}, []string{"status", "--json"})
}

func (w *world) egressFaultComplete() error {
	if err := w.egressGapBytes(strings.Join(func() []string {
		var result []string
		for _, seq := range w.egress.life.sequences {
			result = append(result, fmt.Sprint(seq))
		}
		return result
	}(), " ")); err != nil {
		return err
	}
	for _, result := range w.egress.results[2:] {
		e, _, err := egressLifeEntry(result, w.egress.life.id)
		if err != nil {
			return err
		}
		if e["cleanup"] != "complete" || e["resource"] != "absent" || e["anchor"] != "absent" || len(anyList(e["retry"])) != 0 {
			return errors.New("fresh recovery did not complete all owned cleanup")
		}
	}
	a := w.doctor.engine.egress
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range a.ids {
		if a.containers[id] != nil || a.networks[id] != nil {
			return errors.New("owned resource remains after recovery")
		}
	}
	if !slices.Equal(a.removed, []string{"worker", "proxy", "egress", "internal", "anchor"}) || a.problem != nil {
		return errors.New("recovery removal order or durable authority was invalid")
	}
	return nil
}

func (w *world) egressFaultGap() error {
	gapped := w.egress.lifeFault.name == "gapped copy then proxy remove fails"
	state, exit := "stored", 0
	missing := []any{}
	if gapped {
		state, exit = "gapped", 1
		missing = []any{map[string]int{"first": 3, "last": 4}, map[string]int{"first": 6, "last": 6}}
	}
	want := map[string]any{"path": w.egressGapPath(), "state": state, "records": 3, "missing": missing}
	for _, r := range w.egress.results[2:] {
		e, problems, err := egressLifeEntry(r, w.egress.life.id)
		if err != nil {
			return err
		}
		if r.result.exit != exit || !sameJSON(object(e["egress"])["refusal_log"], want) {
			return errors.New("fresh recovery lost durable refusal evidence or its exit precedence")
		}
		if (!gapped && len(problems) != 0) || (gapped && (len(problems) != 1 || problems[0]["rule"] != "recovery.refusal-log-gap")) {
			return errors.New("completed recovery retained cleanup work or lost the gap diagnosis")
		}
	}
	return nil
}

func (w *world) egressFaultPreserved() error {
	f := w.egress.lifeFault
	after, err := w.egressFaultProtectedInputs()
	if err != nil {
		return err
	}
	for path, before := range f.before {
		if after[path] != before {
			return errors.New("cleanup changed a checkout edit or existing protected input")
		}
	}
	state, err := doctorTree(w.egressStatePath())
	if err != nil || !reflect.DeepEqual(state, f.afterRetry) {
		return errors.New("repeated recovery changed completed protected state")
	}
	data, err := os.ReadFile(w.egressGapPath())
	if err != nil || !bytes.Equal(data, f.log) {
		return errors.New("repeated recovery changed the log")
	}
	for _, r := range w.egress.results[4:] {
		for _, request := range r.journal {
			if request.Method != "GET" && request.Method != "HEAD" {
				return errors.New("repeated recovery mutated the engine")
			}
		}
	}
	a := w.doctor.engine.egress
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.containers[strings.Repeat("e", 64)]; c == nil || object(c["State"])["Running"] != true || a.networks[strings.Repeat("f", 64)] == nil {
		return errors.New("unrelated resources changed")
	}
	return nil
}
