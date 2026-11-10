package features

import (
	"errors"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// Run shares doctor's exposure classification but has no JSON mode, so its
// refusal is read from the plain diagnostic the person sees.
func (w *world) controlRunSteps(sc *godog.ScenarioContext) {
	sc.Step(`^W starts and a fresh P run inspects that location$`, func() {
		f := w.control
		f.startWorker()
		var err error
		f.before, err = f.inventory()
		f.err = errors.Join(f.err, err)
		f.p.deadline = 8 * time.Second
		f.err = errors.Join(f.err, f.p.runEnv(f.p.doctor.cwd, f.p.env(), f.p.bin, "run", "--", "slice-21-workload"))
	})
	sc.Step(`^W has an outstanding owned worker and P run reports approval.state.exposed with exit 1 without requesting reapproval$`, func() error {
		f := w.control
		if err := errors.Join(f.err, f.workers.live(f.workers.first)); err != nil {
			return err
		}
		if err := f.p.runProblem("approval.state.exposed", 1, ""); err != nil {
			return err
		}
		if strings.Contains(f.p.stderr, "run.approval-") || strings.Contains(f.p.stderr, "itos container approve") {
			return errors.New("run reported exposed control state as an approval problem and asked for approval: " + f.p.stderr)
		}
		return nil
	})
	sc.Step(`^P run created no engine resource and changed no control-state entry$`, func() error {
		f := w.control
		return errors.Join(f.unchanged(), f.inspectionOnly())
	})
}
