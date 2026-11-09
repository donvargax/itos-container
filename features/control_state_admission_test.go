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
		if err := errors.Join(f.err, f.workers.err, f.p.approvalSuccess(), f.p.approvalStored("1")); err != nil {
			return err
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
		f.workers.first = f.workers.start("W", "", "")
		f.err = errors.Join(f.err, controlAwaitFile(worker.doctor.env["SLICE_21_READY"]))
	}
	f.err = errors.Join(f.err, f.p.runDoctor(f.p.env(), "approve", "--json", "--fingerprint", f.p.approval.fingerprint))
	var err error
	f.before, err = f.inventory()
	f.err = errors.Join(f.err, err)
	if schedule == "after preflight" {
		f.err = errors.Join(f.err, os.WriteFile(worker.doctor.env["SLICE_21_RELEASE"], []byte("continue"), 0600))
	} else {
		f.workers.first = f.workers.start("W", "", "")
	}
	// Observe either refusal or the forbidden create so the original bug fails
	// at the behavior assertion without waiting for a live workload to time out.
	f.workers.observe(f.workers.first, true)
}
