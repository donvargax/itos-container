package features

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cucumber/godog"
)

func (w *world) controlCollectionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a control-state fixture selecting "([^"]*)" at "([^"]*)" depth in the "([^"]*)" managed tree$`, func(collection, depth, location string) error {
		if err := w.controlSetup("absent", "below W root", "distinct daemons"); err != nil {
			return err
		}
		f := w.control
		root, err := f.collectionRoot(location)
		if err != nil {
			return err
		}
		if location != "absent fixed" {
			if _, err := f.seedBinding(f.fixedRoot()); err != nil {
				return err
			}
			if _, err := f.seedBinding(root); err != nil {
				return err
			}
		}
		state := filepath.Join(root, collection)
		switch depth {
		case "direct":
		case "nested":
			state = filepath.Join(state, "nested")
		default:
			return fmt.Errorf("unknown collection depth %q", depth)
		}
		f.p.doctor.env["XDG_STATE_HOME"] = state
		return f.p.approvalOracle()
	})
	sc.Step(`^fresh commands select collection-prefix siblings of the "([^"]*)" managed tree$`, func(location string) {
		f := w.control
		root, err := f.collectionRoot(location)
		f.err = errors.Join(f.err, err)
		for _, collection := range []string{"sessions-more", "admission-more"} {
			f.p.doctor.env["XDG_STATE_HOME"] = filepath.Join(root, collection)
			f.err = errors.Join(f.err, f.p.approvalOracle())
			f.inspect("approve", "JSON")
			f.err = errors.Join(f.err, f.p.approvalSuccess(), f.p.approvalStored("1"))
			f.inspect("doctor", "JSON")
			f.err = errors.Join(f.err, f.p.exitsWith(0), f.p.approvalStatus("matching"), f.unchanged())
		}
	})
}

func (f *controlFixture) collectionRoot(location string) (string, error) {
	switch location {
	case "absent fixed", "initialized fixed":
		return f.fixedRoot(), nil
	case "enrolled":
		return filepath.Join(f.workers.worlds["W"].doctor.env["XDG_STATE_HOME"], "itos-container"), nil
	default:
		return "", fmt.Errorf("unknown collection location %q", location)
	}
}
