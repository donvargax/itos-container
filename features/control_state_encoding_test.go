package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

// The catalog stores paths as JSON, which replaces an invalid UTF-8 byte with
// U+FFFD. A binding published for such a path would name a different directory,
// so the next transaction would find an enrolled location that is absent and
// refuse every selection on the account. The fixtures name each tree with a
// byte that is not UTF-8 and check that the refusal comes before any state exists.
func (w *world) controlEncodingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a control-state fixture selecting state through "([^"]*)" with a path that is not valid UTF-8$`, func(source string) error {
		if err := w.controlSetup("absent", "below W root", "distinct daemons"); err != nil {
			return err
		}
		f := w.control
		bad := filepath.Join(f.parent, "selected-\xff")
		switch source {
		case "XDG_STATE_HOME":
			f.p.doctor.env["XDG_STATE_HOME"] = bad
		case "HOME fallback":
			delete(f.p.doctor.env, "XDG_STATE_HOME")
			f.p.doctor.env["HOME"] = bad
		default:
			return fmt.Errorf("unknown state source %q", source)
		}
		return f.p.approvalOracle()
	})
	sc.Step(`^a control-state fixture whose account home path is not valid UTF-8$`, func() error {
		if err := w.controlSetup("absent", "first approval", "distinct daemons"); err != nil {
			return err
		}
		f := w.control
		f.home = filepath.Join(f.parent, "account-\xff")
		if err := os.MkdirAll(f.home, 0700); err != nil {
			return err
		}
		f.p.doctor.env["SLICE_21_ACCOUNT_HOME"] = f.home
		f.p.doctor.env["HOME"] = f.home
		return f.p.approvalOracle()
	})
	sc.Step(`^P reports approval.state.location with exit 2 and invalid approval$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalProblem("approval.state.location", 2, ""), f.p.approvalCandidate())
	})
	sc.Step(`^P reports approval.state.unavailable with exit 3 and invalid approval$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalProblem("approval.state.unavailable", 3, ""), f.p.approvalCandidate())
	})
	sc.Step(`^the refusal creates no bootstrap binding or selected-tree entry and preserves existing state$`, func() error {
		return errors.Join(w.control.unchanged(), w.control.inspectionOnly())
	})
	sc.Step(`^fresh commands select the default tree and another checkout's safe state$`, func() {
		f := w.control
		f.p.doctor.env["HOME"] = f.home
		f.p.doctor.env["XDG_STATE_HOME"] = filepath.Dir(f.fixedRoot())
		f.err = errors.Join(f.err, f.p.approvalOracle())
		f.inspect("approve", "JSON")
		f.err = errors.Join(f.err, f.p.approvalSuccess(), f.p.approvalStored("1"))
		f.inspect("doctor", "JSON")
		f.err = errors.Join(f.err, f.p.exitsWith(0), f.p.approvalStatus("matching"), f.unchanged())
		other := f.workers.worlds["W"]
		for _, args := range [][]string{{"doctor", "--json"}, {"approve", "--json", "--fingerprint", other.approval.fingerprint}} {
			f.err = errors.Join(f.err, other.runDoctor(other.env(), args...))
			f.err = errors.Join(f.err, other.exitsWith(0), other.approvalStatus("matching"), other.approvalRecordUnchanged())
		}
	})
}
