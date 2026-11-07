package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) controlPublicationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a safe control-state publication fixture with prior P approval and location fault "([^"]*)"$`, func(fault string) error {
		if err := w.controlSafe("state path shares only a textual prefix with W"); err != nil {
			return err
		}
		f := w.control
		if err := f.p.approvalSeed("4", false); err != nil {
			return err
		}
		f.p.doctor.manifest["containerEnv"] = map[string]string{"APP": "new"}
		if err := f.p.approvalManifest(); err != nil {
			return err
		}
		if err := f.p.approvalOracle(); err != nil {
			return err
		}
		if _, err := f.seedBinding(f.fixedRoot()); err != nil {
			return err
		}
		f.p.doctor.env["SLICE_21_FAULT"] = fault
		return nil
	})
	sc.Step(`^P consents to changed settings while the location fault is active$`, func() { w.control.tracedApproval() })
	sc.Step(`^P reports approval.state.write with exit 75 before creating or replacing an approval record$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalProblem("approval.state.write", 75, ""), f.p.approvalRecordUnchanged())
	})
	sc.Step(`^the prior approval bytes generation inode and mtime remain unchanged and no untracked approval is visible$`, func() error { return w.control.p.approvalRecordUnchanged() })
	sc.Step(`^the fixture clears only the fault and a fresh P process repeats the same consent$`, func() {
		f := w.control
		f.err = errors.Join(f.err, f.captureBootstrap())
		delete(f.p.doctor.env, "SLICE_21_FAULT")
		f.tracedApproval()
	})
	sc.Step(`^P durably enrolls the selected location before publishing the complete next approval generation without discarding earlier location evidence$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalSuccess(), f.p.approvalStored("5"), f.locationDurable(f.p, f.traces), f.retainedBootstrap())
	})
	sc.Step(`^safe independent control-state projects with "([^"]*)" and one preexisting matching approval$`, w.controlSafe)
	sc.Step(`^an unrelated W worker stays active while fresh approval processes use distinct XDG state homes and different child HOME values$`, func() {
		f := w.control
		f.startWorker()
		p := f.p
		for _, project := range []*world{p, f.workers.worlds["W"]} {
			f.p = project
			f.tracedApproval()
			f.results = append(f.results, doctorResult{exit: project.exit, stdout: project.stdout, stderr: project.stderr})
		}
		f.p = p
	})
	sc.Step(`^both selected locations have durable bindings in the same effective-user catalog before successful approval results$`, func() error {
		f := w.control
		if f.err != nil {
			return f.err
		}
		if len(f.results) != 2 {
			return errors.New("missing independent approval results")
		}
		for _, r := range f.results {
			if r.exit != 0 {
				return errors.New("independent approval did not succeed")
			}
		}
		return errors.Join(f.locationDurable(f.p, f.traces), f.locationDurable(f.workers.worlds["W"], f.traces))
	})
	sc.Step(`^both approvals match their independent candidates while the preexisting approval keeps its generation bytes inode and mtime$`, func() error {
		f := w.control
		return errors.Join(f.p.approvalStored("1"), f.workers.worlds["W"].approvalStored("1"), f.workers.worlds["W"].approvalRecordUnchanged())
	})
	sc.Step(`^W retains its original full ID activity and ownership through both publications$`, func() error {
		f := w.control
		return errors.Join(f.workers.live(f.workers.first), f.workers.first.unchanged(), f.inspectionOnly())
	})
	sc.Step(`^all approval processes exit and a fresh doctor selects an empty third state root using the HOME fallback$`, func() {
		f := w.control
		f.err = errors.Join(f.err, f.captureBootstrap())
		f.p.doctor.env["HOME"] = filepath.Join(f.parent, "third-home")
		f.err = errors.Join(f.err, os.Mkdir(f.p.doctor.env["HOME"], 0700))
		delete(f.p.doctor.env, "XDG_STATE_HOME")
		f.inspect("doctor", "JSON")
	})
	sc.Step(`^doctor reports missing approval with exit 0 and writes nothing while both earlier location bindings and approvals remain intact$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.exitsWith(0), f.p.approvalStatus("missing"), f.unchanged(), f.retainedBootstrap())
	})
	sc.Step(`^selecting a state root changes neither the independently calculated candidate fingerprint nor the authority of approvals in other roots$`, func() error {
		f := w.control
		before := f.p.approval.fingerprint
		if err := f.p.approvalOracle(); err != nil {
			return err
		}
		if f.p.approval.fingerprint != before {
			return errors.New("selected control state changed the candidate fingerprint")
		}
		return f.workers.worlds["W"].approvalRecordUnchanged()
	})
}

func (w *world) controlSafe(layout string) error {
	if err := w.controlSetup("absent", "below W root", "distinct daemons"); err != nil {
		return err
	}
	f := w.control
	switch layout {
	case "state path shares only a textual prefix with W":
		f.p.doctor.env["XDG_STATE_HOME"] = f.workers.worlds["W"].doctor.checkout + "-more"
	case "disjoint project paths have equal basenames":
		f.p.doctor.env["XDG_STATE_HOME"] = filepath.Join(f.p.doctor.host, "state")
	default:
		return fmt.Errorf("unknown safe control-state layout %q", layout)
	}
	return f.p.approvalOracle()
}

func (f *controlFixture) locationDurable(w *world, processes []*recoveryProcess) error {
	root := filepath.Dir(filepath.Dir(filepath.Dir(w.approval.record)))
	name := filepath.Join(f.coordination(), approvalHash(root)+".json")
	if root == f.fixedRoot() {
		name = filepath.Join(f.coordination(), "self.json")
	}
	want, err := controlBinding(root, f.home, true)
	if err != nil {
		return err
	}
	record, err := sessionReadRecord(name)
	if err != nil {
		return err
	}
	if !recoveryJSONEqual(record.Bytes, want) {
		return errors.New("location binding does not match independent filesystem identity")
	}
	for _, p := range processes {
		events, err := controlEvents(p.journal)
		if err != nil {
			return err
		}
		var syncs []recoverySync
		for _, e := range events {
			if e.Op == "fsync" && e.Phase == "end" && e.Success {
				syncs = append(syncs, e.recoverySync)
			}
			if e.Phase == "begin" && (e.Op == "create" || e.Op == "rename") && strings.Contains(e.Path, "/approvals/"+w.approval.project+"/") && !recoverySynced(record, name, syncs) {
				return errors.New("approval mutation preceded durable location enrollment")
			}
		}
		if recoverySynced(record, name, syncs) {
			return nil
		}
	}
	return errors.New("location binding lacks successful file and parent fsync evidence")
}
