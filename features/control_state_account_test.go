package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

func (w *world) controlAccountSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a read-only production account lookup fixture whose account database is "([^"]*)"$`, func(database string) error {
		if err := w.controlSetup("absent", "first approval", "shared daemon"); err != nil {
			return err
		}
		f := w.control
		f.condition = database
		f.p.doctor.env["SLICE_21_ACCOUNT_PROBE"] = "1"
		f.p.doctor.env["SLICE_21_FAULT"] = "account database " + database
		for _, name := range []string{"first", "second"} {
			if err := os.Mkdir(filepath.Join(f.home, name), 0700); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^two fresh CGO-disabled account lookups use different HOME USER and LOGNAME values$`, func() {
		f := w.control
		var err error
		f.before, err = f.inventory()
		f.err = errors.Join(f.err, err)
		for _, name := range []string{"first", "second"} {
			f.p.doctor.env["HOME"] = filepath.Join(f.home, name)
			f.p.doctor.env["USER"], f.p.doctor.env["LOGNAME"] = name, name
			process := f.traceCommand("account-probe")
			<-process.done
			f.traces = append(f.traces, process)
		}
	})
	sc.Step(`^both lookups report state.unavailable without selecting a home or creating control state$`, func() error {
		f := w.control
		if f.err != nil {
			return f.err
		}
		for _, process := range f.traces {
			if process.err != nil || process.exit != 0 {
				return fmt.Errorf("account probe failed: exit %d: %v: %s", process.exit, process.err, process.errout.bytes())
			}
			events, err := controlEvents(process.journal)
			if err != nil {
				return err
			}
			faulted := false
			for _, event := range events {
				if event.Phase == "end" && ((f.condition == "empty" && event.Op == "account-read" && event.Success) ||
					(f.condition == "unreadable" && event.Op == "account-open" && !event.Success)) {
					faulted = true
				}
			}
			if !faulted {
				return errors.New("account database fault was not observed")
			}
			var result struct{ Home, Failure string }
			if err := json.Unmarshal(process.out.bytes(), &result); err != nil {
				return err
			}
			if result.Failure != "state.unavailable" || result.Home != "" {
				return fmt.Errorf("account lookup selected an unproven home: failure=%q, home selected=%t", result.Failure, result.Home != "")
			}
		}
		return f.unchanged()
	})
}
