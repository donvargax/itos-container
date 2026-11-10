package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) controlLifecycleSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a control-state fixture whose W reservation will be "([^"]*)" and whose P managed location is below W$`, func(condition string) error {
		if err := w.controlSetup("absent", "below W root", "distinct daemons"); err != nil {
			return err
		}
		f := w.control
		f.condition = condition
		a := f.workers.engines["W"]
		a.faultRoot = f.workers.worlds["W"].doctor.checkout
		switch condition {
		case "DELETE acknowledged but full worker ID still present":
			a.condition = "workload exited with successful DELETE but worker still present"
		case "resource absent but completion publication failed":
			a.condition = "resource removed with session completion fsync failed"
		case "preparing after registration before create", "controller absent with worker running", "controller absent with resource observation unavailable", "one of two same-root sessions completed":
		default:
			return fmt.Errorf("unknown reservation condition %q", condition)
		}
		return nil
	})
	sc.Step(`^W reaches that condition and P invokes "([^"]*)" with valid approval inputs from a different state root$`, func(command string) {
		f := w.control
		f.reachCondition()
		f.inspect(command, "JSON")
	})
	sc.Step(`^the exposing reservation is evidenced and P reports approval.state.exposed with exit 1 without trusting or publishing approval$`, func() error {
		f := w.control
		if err := f.reservation(); err != nil {
			return err
		}
		return f.p.approvalProblem("approval.state.exposed", 1, "")
	})
	sc.Step(`^P leaves W ownership evidence and all existing approval bytes unchanged$`, func() error {
		return errors.Join(w.control.unchanged(), w.control.workers.first.unchanged())
	})
	sc.Step(`^the fixture clears its fault and product recovery completes every W reservation before fresh P approval$`, func() {
		f := w.control
		a := f.workers.engines["W"]
		a.mu.Lock()
		a.condition = ""
		a.mu.Unlock()
		for _, r := range []*overlapRun{f.workers.first, f.workers.second} {
			if r != nil && r.stopEntry.Cleanup != "complete" {
				f.workers.stop(r)
			}
		}
		f.inspect("approve", "JSON")
	})
	sc.Step(`^P publishes matching approval only after durable completion and retains W approvals snapshots and completed recovery records$`, func() error {
		f := w.control
		if err := errors.Join(f.err, f.workers.err, f.p.approvalSuccess(), f.p.approvalStored("1")); err != nil {
			return err
		}
		for _, r := range []*overlapRun{f.workers.first, f.workers.second} {
			if r == nil {
				continue
			}
			if f.condition == "preparing after registration before create" {
				if r.stopEntry.State != "stopped" || r.stopEntry.Cleanup != "complete" || r.stopEntry.WorkloadExit != nil || f.workers.creates(r) != 0 {
					return errors.New("pre-create cancellation did not durably release the reservation")
				}
				intent, err := sessionReadRecord(r.intent)
				if err != nil {
					return err
				}
				data, err := sessionDecodeRecord(intent.Bytes)
				if err != nil || data["stage"] != "cancelled" {
					return errors.New("pre-create intent was not cancelled")
				}
				if err := recoveryDurableFile(r.intent, data, []*recoveryProcess{r.stop}); err != nil {
					return err
				}
			} else if err := f.workers.complete(r); err != nil {
				return err
			}
			snapshot, err := sessionReadRecord(r.w.sessionSnapshotPath())
			if err != nil || snapshot != r.snapshot {
				return errors.New("completion changed the frozen snapshot")
			}
			if err := r.w.approvalRecordUnchanged(); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^a control-state review fixture with an absent P approval and a prospective location below W$`, func() error {
		return w.controlSetup("absent", "below W root", "distinct daemons")
	})
	sc.Step(`^P shows its terminal review and W starts from another state root before the fixture answers yes$`, func() { w.control.reviewRace() })
	sc.Step(`^W is independently active before consent and P reports approval.state.exposed with exit 1 after consent$`, func() error { return w.control.exposed() })
	sc.Step(`^the unanswered review created no coordination or approval state$`, func() error {
		if !reflect.DeepEqual(w.control.reviewBefore, w.control.reviewAfter) {
			return errors.New("unanswered review changed control state")
		}
		return nil
	})
	sc.Step(`^the refused approval adds no state beneath W and neither changes nor stops W$`, func() error {
		f := w.control
		return errors.Join(f.unchanged(), f.workers.first.unchanged(), f.inspectionOnly())
	})
	sc.Step(`^a control-state fixture configured to pause P publication after final reinspection with "([^"]*)"$`, func(engines string) error {
		if err := w.controlSetup("absent", "below W root", engines); err != nil {
			return err
		}
		f := w.control
		f.p.doctor.env["SLICE_21_BOUNDARY"] = "before publication"
		f.p.doctor.env["SLICE_21_READY"] = filepath.Join(f.parent, "slice-21-ready")
		f.p.doctor.env["SLICE_21_RELEASE"] = filepath.Join(f.parent, "slice-21-release")
		return nil
	})
	sc.Step(`^P pauses at that barrier and W becomes durably admitted from another state root before P resumes$`, func() { w.control.publicationRace() })
	sc.Step(`^the admitted W reservation is evidenced and resumed P reports approval.state.exposed with exit 1 before any selected-state write$`, func() error {
		return errors.Join(w.control.exposed(), w.control.unchanged())
	})
	sc.Step(`^the race leaves no P approval or location enrollment beneath W and preserves W original ownership$`, func() error {
		f := w.control
		return errors.Join(f.unchanged(), f.workers.first.unchanged(), f.inspectionOnly())
	})
}

func (f *controlFixture) reachCondition() {
	o := f.workers
	boundary, fault := "", ""
	if f.condition == "preparing after registration before create" {
		boundary = "before create"
	}
	if f.condition == "resource absent but completion publication failed" {
		fault = "session completion parent fsync"
	}
	o.first = o.start("W", boundary, fault)
	r := o.first
	if boundary != "" {
		if r.probe != nil {
			r.paused = r.probe.await()
			r.discover()
		}
	} else if strings.HasPrefix(f.condition, "controller absent") || f.condition == "one of two same-root sessions completed" {
		o.observe(r, false)
		if strings.HasPrefix(f.condition, "controller absent") && r.active() {
			o.remember(r.process.killController())
			r.process.wait()
		}
		if f.condition == "controller absent with resource observation unavailable" {
			a := o.engines["W"]
			a.mu.Lock()
			a.condition = f.condition
			a.mu.Unlock()
		}
		if f.condition == "one of two same-root sessions completed" {
			o.second = o.start("W", "", "")
			o.observe(o.second, false)
			o.checkpoint(o.second)
			o.stop(r)
		}
	} else {
		o.remember(overlapAwait("faulted worker controller returns", 40*time.Second, r.ended))
		_ = r.result()
		r.discover()
	}
	o.checkpoint(r)
}

func (f *controlFixture) reservation() error {
	o, r := f.workers, f.workers.first
	if err := errors.Join(f.err, o.err); err != nil {
		return err
	}
	if f.condition == "preparing after registration before create" {
		if o.creates(r) != 0 {
			return errors.New("preparing worker already created")
		}
		return o.prepared()
	}
	if !offlineHex(r.id) || !offlineHex(r.worker) || r.public.Bytes == "" || r.owned.Bytes == "" {
		return errors.New("original full-ID reservation was not retained")
	}
	if f.condition == "one of two same-root sessions completed" {
		return errors.Join(o.complete(r), o.live(o.second))
	}
	if strings.HasPrefix(f.condition, "controller absent") {
		if !r.ended() {
			return errors.New("original controller is still present")
		}
		return o.live(r)
	}
	deleted := false
	for _, q := range r.engine.journal() {
		if q.method == "DELETE" && q.path == "/containers/"+r.worker && q.status == 204 {
			deleted = true
		}
	}
	if !deleted {
		return errors.New("worker never reached acknowledged removal")
	}
	r.engine.mu.Lock()
	removed := r.engine.workers[r.worker].removed
	r.engine.mu.Unlock()
	if removed != (f.condition == "resource absent but completion publication failed") {
		return errors.New("worker presence does not match the injected condition")
	}
	return nil
}

func (f *controlFixture) reviewRace() {
	var err error
	f.reviewBefore, err = f.inventory()
	f.err = errors.Join(f.err, err)
	p, err := f.p.startApproval("plain", "", "y")
	if err != nil {
		f.err = errors.Join(f.err, err)
		return
	}
	f.reviewAfter, err = f.inventory()
	f.err = errors.Join(f.err, err)
	if !strings.Contains(p.review, "y/N") {
		f.err = errors.Join(f.err, errors.New("P never showed its terminal review"))
	}
	f.startWorker()
	f.err = errors.Join(f.err, f.workers.live(f.workers.first))
	f.before, err = f.inventory()
	f.err = errors.Join(f.err, err)
	p.answer("y")
	f.p.finishApproval(p)
}

func (f *controlFixture) publicationRace() {
	p := f.p.recoveryStart("approve", "--json", "--fingerprint", f.p.approval.fingerprint)
	f.err = errors.Join(f.err, overlapAwait("P enters publication barrier", 12*time.Second, func() bool {
		data, _ := os.ReadFile(f.p.doctor.env["SLICE_21_READY"])
		return string(data) == "ready"
	}))
	f.startWorker()
	var err error
	f.before, err = f.inventory()
	f.err = errors.Join(f.err, err, os.WriteFile(f.p.doctor.env["SLICE_21_RELEASE"], []byte("continue"), 0600))
	p.wait()
	f.err = errors.Join(f.err, p.err)
	f.p.exit, f.p.stdout, f.p.stderr = p.exit, string(p.out.bytes()), string(p.errout.bytes())
}
