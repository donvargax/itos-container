package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) checkoutBudgetSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the checkout enumeration fixture exceeds the "([^"]*)" budget$`, w.checkoutOverBudget)
	sc.Step(`^"([^"]*)" runs and the checkout exceeds the depth budget before its image response$`, w.checkoutBudgetRace)
	sc.Step(`^the checkout budget action reached the image barrier$`, func() error {
		if !w.hardlinks.reached {
			return fmt.Errorf("checkout budget action did not reach the image barrier")
		}
		return nil
	})
}

func (w *world) checkoutOverBudget(kind string) error {
	path := filepath.Join(w.doctor.checkout, "slice-20-budget")
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	switch kind {
	case "entries":
		for i := 0; i < 100001; i++ {
			if err := os.WriteFile(filepath.Join(path, fmt.Sprint(i)), nil, 0600); err != nil {
				return err
			}
		}
	case "depth":
		for i := 1; i < 65; i++ {
			path = filepath.Join(path, "d")
			if err := os.Mkdir(path, 0700); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown checkout budget %q", kind)
	}
	return nil
}

func (w *world) checkoutBudgetRace(command string) error {
	h := w.hardlinks
	h.command = command
	w.doctor.engine.paused = true
	args := []string{command, "--json"}
	if command == "run" {
		args = []string{"run", "--", "/bin/true"}
	}
	p, err := w.startRun(w.env(), args...)
	if err != nil {
		return err
	}
	select {
	case <-p.done:
	case <-w.doctor.engine.reached:
		h.reached = true
		err = w.checkoutOverBudget("depth")
		if err == nil {
			err = w.hardlinkCheckpoint()
		}
	case <-time.After(6 * time.Second):
	}
	w.doctor.engine.unpause()
	return errors.Join(err, w.hardlinkFinish(p))
}
