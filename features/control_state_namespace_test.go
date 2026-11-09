package features

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cucumber/godog"
)

func (w *world) controlNamespaceSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a control-state fixture with an "([^"]*)" catalog and XDG_STATE_HOME "([^"]*)" inside its metadata namespace$`, func(catalog, selection string) error {
		if err := w.controlSetup("absent", "below W root", "distinct daemons"); err != nil {
			return err
		}
		f := w.control
		switch catalog {
		case "initialized":
			if _, err := f.seedBinding(f.fixedRoot()); err != nil {
				return err
			}
		case "absent":
		default:
			return fmt.Errorf("unknown catalog state %q", catalog)
		}
		state := f.coordination()
		switch selection {
		case "catalog":
		case "catalog child":
			state = filepath.Join(state, "nested")
		default:
			return fmt.Errorf("unknown catalog selection %q", selection)
		}
		f.p.doctor.env["XDG_STATE_HOME"] = state
		return f.p.approvalOracle()
	})
	sc.Step(`^P inspects that selection with "([^"]*)" using valid approval inputs$`, func(command string) {
		w.control.inspect(command, "JSON")
	})
	sc.Step(`^P reports approval.state.path with exit 1 and invalid approval$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalProblem("approval.state.path", 1, ""), f.p.approvalCandidate())
	})
	sc.Step(`^the namespace refusal creates no bootstrap binding or selected-tree entry and preserves existing state$`, func() error {
		return errors.Join(w.control.unchanged(), w.control.inspectionOnly())
	})
	sc.Step(`^fresh commands select the default tree a catalog-prefix sibling and another checkout's safe state$`, func() {
		f := w.control
		for _, state := range []string{filepath.Dir(f.fixedRoot()), f.coordination() + "-more"} {
			f.p.doctor.env["XDG_STATE_HOME"] = state
			f.err = errors.Join(f.err, f.p.approvalOracle())
			f.inspect("approve", "JSON")
			f.err = errors.Join(f.err, f.p.approvalSuccess(), f.p.approvalStored("1"))
			f.inspect("doctor", "JSON")
			f.err = errors.Join(f.err, f.p.exitsWith(0), f.p.approvalStatus("matching"), f.unchanged())
		}
		other := f.workers.worlds["W"]
		for _, args := range [][]string{{"doctor", "--json"}, {"approve", "--json", "--fingerprint", other.approval.fingerprint}} {
			f.err = errors.Join(f.err, other.runDoctor(other.env(), args...))
			f.err = errors.Join(f.err, other.exitsWith(0), other.approvalStatus("matching"), other.approvalRecordUnchanged())
		}
	})
	sc.Step(`^all safe selections remain usable without engine mutation or checkout execution$`, func() error {
		return errors.Join(w.control.err, w.control.inspectionOnly())
	})
}
