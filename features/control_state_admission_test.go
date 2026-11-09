package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

func (w *world) controlAdmissionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^P publishes approval before W admission using the "([^"]*)" schedule$`, func(schedule string) {
		w.control.publicationBeforeAdmission(schedule)
	})
	sc.Step(`^P approval succeeds and W reports "([^"]*)" with exit 1 before registration or engine mutation$`, func(rule string) error {
		f := w.control
		if err := errors.Join(f.err, f.workers.err, f.p.approvalSuccess(), f.locationDurable(f.p, []*recoveryProcess{f.publisher})); err != nil {
			return err
		}
		a := f.p.approval
		record, err := sessionReadRecord(a.record)
		if err != nil {
			return err
		}
		if !recoveryJSONEqual(record.Bytes, map[string]any{"schema": 1, "project": a.project, "generation": "1", "fingerprint": a.fingerprint, "settings": a.settings}) {
			return errors.New("publisher did not retain the expected approval record")
		}
		r := f.workers.first
		got := r.result()
		r.w.exit, r.w.stdout, r.w.stderr = got.exit, got.stdout, got.stderr
		if err := r.w.runProblem(rule, 1, ""); err != nil {
			return err
		}
		if err := errors.Join(r.process.err, f.noContenderAuthority()); err != nil {
			return err
		}
		for _, engine := range f.workers.engines {
			for _, request := range engine.journal() {
				if request.method != "GET" && request.method != "HEAD" {
					return fmt.Errorf("refused admission mutated engine: %s %s", request.method, request.path)
				}
			}
		}
		return nil
	})
	sc.Step(`^W preserves the enrolled catalog approvals and both projects without creating launch authority$`, func() error {
		return errors.Join(w.control.unchanged(), w.control.inspectionOnly())
	})
}

func (f *controlFixture) publicationBeforeAdmission(schedule string) {
	worker := f.workers.worlds["W"]
	if schedule == "after preflight" {
		worker.doctor.env["SLICE_21_BOUNDARY"] = "before admission"
		worker.doctor.env["SLICE_21_READY"] = filepath.Join(f.parent, "slice-21-admission-ready")
		worker.doctor.env["SLICE_21_RELEASE"] = filepath.Join(f.parent, "slice-21-admission-release")
		f.startAdmissionContender()
		f.err = errors.Join(f.err, controlAwaitFile(worker.doctor.env["SLICE_21_READY"]))
	}
	f.publisher = f.traceCommand("approve", "--json", "--fingerprint", f.p.approval.fingerprint)
	f.publisher.wait()
	f.err = errors.Join(f.err, f.publisher.err)
	f.p.exit, f.p.stdout, f.p.stderr = f.publisher.exit, string(f.publisher.out.bytes()), string(f.publisher.errout.bytes())
	var err error
	f.before, err = f.inventory()
	f.err = errors.Join(f.err, err)
	if schedule == "after preflight" {
		f.err = errors.Join(f.err, os.WriteFile(worker.doctor.env["SLICE_21_RELEASE"], []byte("continue"), 0600))
	} else {
		f.startAdmissionContender()
	}
	// Observe either refusal or the forbidden create so the original bug fails
	// at the behavior assertion without waiting for a live workload to time out.
	f.workers.observe(f.workers.first, true)
}

func (f *controlFixture) startAdmissionContender() {
	publisher := f.p
	f.p = f.workers.worlds["W"]
	r := &overlapRun{w: f.p, engine: f.workers.engines["W"]}
	r.process = f.traceCommand("run", "--", "slice-21-workload")
	f.workers.first = r
	f.workers.runs = append(f.workers.runs, r)
	f.p = publisher
}
