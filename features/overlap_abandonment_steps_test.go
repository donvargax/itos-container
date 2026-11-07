package features

import (
	"errors"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) overlapAbandonmentSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an overlap fixture interrupted by "([^"]*)" before any worker create$`, func(fault string) error {
		if err := w.overlapSetup("child", "shared daemon", false); err != nil {
			return err
		}
		w.overlap.condition = fault
		return nil
	})
	sc.Step(`^the ancestor registration fails or its controller dies and any visible session is stopped before a descendant run$`, func() {
		f := w.overlap
		boundary, fault := "", f.condition
		if fault == "reservation before registration" {
			boundary, fault = fault, ""
		}
		f.first = f.start("ancestor", boundary, fault)
		if boundary != "" {
			f.awaitPrecreate()
			f.killPrecreate()
		} else {
			f.remember(overlapAwait("registration failure returns", 15*time.Second, f.first.ended))
			got := f.first.result()
			if got.exit == 0 {
				f.remember(errors.New("registration fault did not fail the run"))
			}
		}
		// Discover a visible record even when registration's parent fsync failed.
		f.first.paused = true
		f.first.discover()
		if f.first.id != "" {
			f.stop(f.first)
		}
		f.contender = f.start("descendant", "", "")
		f.observe(f.contender, false)
	})
	sc.Step(`^the descendant workload starts and the interrupted ancestor has sent no engine mutation$`, func() error {
		return w.overlap.cancelledAncestor()
	})
	sc.Step(`^an overlap fixture with a pre-create controller that will be "([^"]*)"$`, func(controller string) error {
		if err := w.overlapSetup("child", "shared daemon", false); err != nil {
			return err
		}
		w.overlap.condition = controller
		return nil
	})
	sc.Step(`^the ancestor pauses before create and product stop completes it before a descendant run$`, func() {
		f := w.overlap
		f.first = f.start("ancestor", "before create", "")
		f.awaitPrecreate()
		f.first.discover()
		if f.condition == "killed" {
			f.killPrecreate()
		}
		f.stop(f.first)
		f.contender = f.start("descendant", "", "")
		f.observe(f.contender, false)
	})
	sc.Step(`^the descendant workload starts after durable pre-create cancellation with no invented workload exit$`, func() error {
		f := w.overlap
		if err := f.live(f.contender); err != nil {
			return fmt.Errorf("pre-create stop stranded checkout: %w; contender=%+v", err, f.contender.result())
		}
		e := f.first.stopEntry
		if f.first.stop == nil || f.first.stop.exit != 0 || e.Cleanup != "complete" || e.State != "stopped" || e.Resource != "absent" || e.WorkloadExit != nil {
			return errors.New("pre-create stop did not report complete cleanup with a null workload exit")
		}
		return f.preserved()
	})
	sc.Step(`^the original pre-create controller is released if still present$`, func() {
		f := w.overlap
		if f.condition == "resumed" {
			f.remember(f.first.probe.resume())
			f.remember(overlapAwait("cancelled controller returns", 15*time.Second, f.first.ended))
			_ = f.first.result()
		}
	})
	sc.Step(`^the cancelled ancestor sends no engine mutation while the descendant remains active$`, func() error {
		return w.overlap.cancelledAncestor()
	})
}

func (f *overlapFixture) awaitPrecreate() {
	if f.first.probe == nil || !f.first.probe.await() {
		f.remember(errors.New("pre-create barrier was not reached"))
	}
	f.first.paused = true
}

func (f *overlapFixture) killPrecreate() {
	if f.first.process != nil {
		f.remember(f.first.process.cmd.Process.Kill())
		f.first.process.wait()
	}
}

func (f *overlapFixture) cancelledAncestor() error {
	if err := f.live(f.contender); err != nil {
		return fmt.Errorf("abandoned registration stranded checkout: %w; contender=%+v", err, f.contender.result())
	}
	for _, q := range f.first.engine.journal() {
		if q.pid == f.first.pid() && q.method != "GET" && q.method != "HEAD" {
			return errors.New("cancelled ancestor sent an engine mutation")
		}
	}
	return f.preserved()
}
