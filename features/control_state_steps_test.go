package features

import (
	"errors"

	"github.com/cucumber/godog"
)

func (w *world) controlStateSteps(sc *godog.ScenarioContext) {
	w.controlAccountSteps(sc)
	w.controlLockSteps(sc)
	w.controlBootstrapRaceSteps(sc)
	w.controlLifecycleSteps(sc)
	w.controlBootstrapSteps(sc)
	w.controlTrackingSteps(sc)
	w.controlPublicationSteps(sc)
	w.controlExclusionSteps(sc)
	w.controlAdmissionSteps(sc)
	w.controlNamespaceSteps(sc)
	sc.Step(`^a control-state fixture with P approval "([^"]*)" and managed location "([^"]*)" relative to W on "([^"]*)"$`, w.controlSetup)
	sc.Step(`^W starts and a fresh P doctor inspects that location in "([^"]*)"$`, func(format string) {
		w.control.startWorker()
		w.control.inspect("doctor", format)
	})
	sc.Step(`^W starts and P approve receives its independently calculated exact fingerprint in "([^"]*)"$`, func(format string) {
		w.control.startWorker()
		w.control.inspect("approve", format)
	})
	sc.Step(`^W has an outstanding owned worker and P reports approval.state.exposed with exit 1 instead of usable approval$`, func() error { return w.control.exposed() })
	sc.Step(`^the doctor invocation leaves all checkout control-state and outside-target entries unchanged$`, func() error { return w.control.unchanged() })
	sc.Step(`^a fresh P doctor selects a missing safe textual-prefix state root while W remains active$`, func() { w.control.safeDoctor() })
	sc.Step(`^doctor reports missing approval with exit 0 without enrolling or creating that safe location$`, func() error { return w.control.missing() })
	sc.Step(`^neither doctor invocation mutates an engine or executes checkout code$`, func() error { return w.control.inspectionOnly() })
	sc.Step(`^P creates no managed directory location claim lock file or approval temp file and preserves any prior approval exactly$`, func() error {
		return errors.Join(w.control.unchanged(), w.control.p.approvalRecordUnchanged())
	})
	sc.Step(`^W retains its full ID activity and original protected ownership without an approve-triggered engine mutation$`, func() error {
		f := w.control
		return errors.Join(f.workers.live(f.workers.first), f.workers.first.unchanged(), f.inspectionOnly())
	})
}
