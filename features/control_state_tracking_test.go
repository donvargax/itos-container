package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) controlTrackingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a control-state fixture with shared tracking defect "([^"]*)" outside P active approval record$`, func(defect string) error { return w.controlTrackingDefect(defect) })
	sc.Step(`^fresh P doctor and exact-fingerprint approve inspect the defect as JSON$`, func() {
		f := w.control
		for _, args := range [][]string{{"doctor", "--json"}, {"approve", "--json", "--fingerprint", f.p.approval.fingerprint}} {
			var err error
			f.before, err = f.inventory()
			f.err = errors.Join(f.err, err)
			p := f.traceCommand(args...)
			p.wait()
			f.err = errors.Join(f.err, p.err, f.unchanged())
			f.results = append(f.results, doctorResult{exit: p.exit, stdout: string(p.out.bytes()), stderr: string(p.errout.bytes())})
		}
	})
	sc.Step(`^both commands report "([^"]*)" with exit (\d+) and invalid approval without a successful publication$`, func(rule string, exit int) error {
		f := w.control
		if f.err != nil {
			return f.err
		}
		if len(f.results) != 2 {
			return errors.New("both tracking inspections were not captured")
		}
		for _, r := range f.results {
			f.p.exit, f.p.stdout, f.p.stderr = r.exit, r.stdout, r.stderr
			if err := f.p.approvalProblem(rule, exit, ""); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^both commands preserve all state defects targets and prior records without repair or engine mutation$`, func() error {
		f := w.control
		return errors.Join(f.err, f.unchanged(), f.p.approvalRecordUnchanged(), f.inspectionOnly())
	})
	sc.Step(`^P invokes policy-only and manifest-only doctor against the same fixture$`, func() {
		f := w.control
		f.results = nil
		f.p.doctor.env["SLICE_21_ACCOUNT_HOME"] = "unavailable"
		for _, mode := range []string{"--policy-only", "--manifest-only"} {
			lookupLog := filepath.Join(f.parent, "slice-21-home-lookups"+mode)
			f.p.doctor.env["SLICE_21_LOOKUP_LOG"] = lookupLog
			p := f.traceCommand("doctor", mode, "--json")
			p.wait()
			f.err = errors.Join(f.err, p.err)
			if _, err := os.Lstat(lookupLog); !errors.Is(err, os.ErrNotExist) {
				f.err = errors.Join(f.err, errors.New("file-only doctor resolved the account home"))
			}
			events, err := controlEvents(p.journal)
			f.err = errors.Join(f.err, err)
			for _, e := range events {
				if strings.HasPrefix(e.Path, f.home+"/") || e.Path == f.home {
					f.err = errors.Join(f.err, errors.New("file-only doctor accessed shared control state"))
				}
			}
			f.results = append(f.results, doctorResult{exit: p.exit, stdout: string(p.out.bytes()), stderr: string(p.errout.bytes())})
		}
	})
	sc.Step(`^both file-only inspections succeed without account-home lookup or shared-state access$`, func() error {
		f := w.control
		if f.err != nil {
			return f.err
		}
		if len(f.results) != 2 {
			return errors.New("missing file-only inspections")
		}
		for _, r := range f.results {
			if r.exit != 0 {
				return errors.New("file-only inspection consulted unavailable account state")
			}
		}
		return f.unchanged()
	})
}

func (f *controlFixture) seedBinding(path string) (string, error) {
	if err := os.MkdirAll(f.coordination(), 0700); err != nil {
		return "", err
	}
	name := approvalHash(path) + ".json"
	if path == f.fixedRoot() {
		name = "self.json"
	}
	record, err := controlBinding(path, f.home, true)
	if err != nil {
		return "", err
	}
	name = filepath.Join(f.coordination(), name)
	return name, os.WriteFile(name, approvalJSON(record), 0600)
}

func (w *world) controlTrackingDefect(defect string) error {
	if err := w.controlSetup("absent", "below W root", "distinct daemons"); err != nil {
		return err
	}
	f := w.control
	f.p.doctor.env["XDG_STATE_HOME"] = filepath.Join(f.p.doctor.host, "state")
	if err := f.p.approvalSeed("4", false); err != nil {
		return err
	}
	if defect == "host account home lookup is unavailable" {
		f.p.doctor.env["SLICE_21_ACCOUNT_HOME"] = "unavailable"
		return nil
	}
	if defect == "catalog is absent beside selected legacy session state" {
		if err := f.p.sessionSeedSnapshot(); err != nil {
			return err
		}
		f.workers.firstName = "P"
		return f.workers.seedLegacy()
	}
	if defect == "coordination anchor is below P checkout" {
		f.home = filepath.Join(f.p.doctor.checkout, "account-home")
		f.p.doctor.env["SLICE_21_ACCOUNT_HOME"] = f.home
	}
	self, err := f.seedBinding(f.fixedRoot())
	if err != nil {
		return err
	}
	other := filepath.Join(f.workers.worlds["W"].doctor.env["XDG_STATE_HOME"], "itos-container")
	location, err := f.seedBinding(other)
	if err != nil {
		return err
	}
	switch defect {
	case "coordination ancestor is a symlink":
		old := filepath.Join(f.home, ".local")
		target := filepath.Join(f.home, "outside-target")
		if err := os.Rename(old, target); err != nil {
			return err
		}
		return os.Symlink(target, old)
	case "coordination anchor is below P checkout":
		return nil
	case "cataloged root was replaced at the same path":
		if err := os.Rename(other, other+"-original"); err != nil {
			return err
		}
		return os.Mkdir(other, 0700)
	case "location record has a second hard link":
		return os.Link(location, filepath.Join(f.home, "outside-target"))
	case "coordination directory permits another user to write":
		return os.Chmod(f.coordination(), 0770)
	case "location record binds another effective UID":
		record, err := controlBinding(other, f.home, true)
		if err != nil {
			return err
		}
		record["uid"] = strconv.Itoa(os.Geteuid() + 1)
		return os.WriteFile(location, approvalJSON(record), 0600)
	case "location record is truncated":
		return os.WriteFile(location, []byte(`{"schema":`), 0600)
	case "previously cataloged root is absent":
		return os.Rename(other, other+"-absent")
	case "cataloged reservation cannot be read":
		f.p.doctor.env["SLICE_21_FAULT"] = defect
		p := f.workers.worlds["W"]
		id := strings.Repeat("b", 64)
		record := map[string]any{"schema": 1, "root": p.doctor.checkout, "id": id, "intent": "intents/" + p.approval.project + "/" + strings.Repeat("a", 64) + ".json", "complete": false}
		name := approvalHash(map[string]any{"schema": 1, "uid": p.approval.uid, "root": p.recoveryIndexKey() + "/" + id}) + ".json"
		return writeManifestFixture(filepath.Join(other, "admission", name), approvalJSON(record), 0600)
	default:
		return fmt.Errorf("unknown tracking defect %q (%s)", defect, filepath.Base(self))
	}
}
