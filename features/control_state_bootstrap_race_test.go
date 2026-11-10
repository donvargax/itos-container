package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) controlBootstrapRaceSteps(sc *godog.ScenarioContext) {
	var lockInfo os.FileInfo
	var contended bool
	sc.Step(`^P consents and a peer creates and holds the private lock after P completes unlocked bootstrap inspection$`, func() error {
		f := w.control
		ready, release := filepath.Join(f.parent, "slice-21-bootstrap-ready"), filepath.Join(f.parent, "slice-21-bootstrap-release")
		f.p.doctor.env["SLICE_21_BOUNDARY"] = "after bootstrap inspection"
		f.p.doctor.env["SLICE_21_READY"], f.p.doctor.env["SLICE_21_RELEASE"] = ready, release
		f.publisher = f.traceCommand("approve", "--json", "--fingerprint", f.p.approval.fingerprint)
		if err := controlAwaitFile(ready); err != nil {
			return err
		}
		// Model a peer between exclusive lock creation and self-binding. Its
		// independent descriptor holds the actual flock, never a fake result.
		if err := os.MkdirAll(f.coordination(), 0700); err != nil {
			return err
		}
		path := filepath.Join(f.coordination(), "control.lock")
		if err := os.WriteFile(path, nil, 0600); err != nil {
			return err
		}
		var err error
		lockInfo, err = os.Stat(path)
		if err != nil {
			return err
		}
		unlock, err := f.holdControlLock(path)
		if err != nil {
			return err
		}
		defer unlock()
		if err := os.WriteFile(release, []byte("continue"), 0600); err != nil {
			return err
		}
		f.err = overlapAwait("writer contention or return", 5*time.Second, func() bool {
			events, _ := controlEvents(f.publisher.journal)
			for _, e := range events {
				if e.Op == "flock" && e.Path == path && e.Phase == "end" && !e.Success && e.Flags&2 != 0 {
					contended = true
					return true
				}
			}
			select {
			case <-f.publisher.done:
				return true
			default:
				return false
			}
		})
		return nil
	})
	sc.Step(`^P joins the peer's exclusion and publishes generation 1 after release without replacing the private lock$`, func() error {
		f := w.control
		f.publisher.wait()
		f.p.exit, f.p.stdout, f.p.stderr = f.publisher.exit, string(f.publisher.out.bytes()), string(f.publisher.errout.bytes())
		if !contended {
			return fmt.Errorf("writer did not join the peer lock: exit %d, %s %s", f.p.exit, f.p.stdout, f.p.stderr)
		}
		after, err := os.Stat(filepath.Join(f.coordination(), "control.lock"))
		if err != nil || !os.SameFile(lockInfo, after) {
			return fmt.Errorf("writer replaced the peer lock: %v", err)
		}
		return errors.Join(f.err, f.publisher.err, f.p.approvalSuccess(), f.p.approvalStored("1"), f.selfBinding(), f.inspectionOnly())
	})
}
