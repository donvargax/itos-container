package features

import (
	"fmt"

	"github.com/cucumber/godog"
)

func (w *world) recoveryNativeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a CI recovery checkout with the cached image a marker workload and a removal-only transient fault$`, func() error {
		return w.recoveryNativeFixture("retry")
	})
	sc.Step(`^the product runs the workload to exit 7 and a fresh product process executes its printed stop after the fault is cleared$`, w.recoveryRetryAction)
	sc.Step(`^the run reports exit 7 with a registered retry and the stop returns zero after native full-ID absence and durable completion$`, w.recoveryRetrySucceeded)
	sc.Step(`^status retains workload exit 7 while checkout edits approvals snapshots and unrelated native resources survive$`, func() error {
		if err := w.recoveryCompleted(); err != nil {
			return err
		}
		return w.recoveryPreserved()
	})
	sc.Step(`^the product creates no replacement worker and harness cleanup is not counted as product completion$`, w.recoveryOneLaunch)

	sc.Step(`^a CI recovery checkout with the cached image and a long-lived heartbeat workload$`, func() error {
		return w.recoveryNativeFixture("loss")
	})
	sc.Step(`^the product starts the workload, the harness kills only its controller, then fresh product status and stop processes run$`, w.recoveryLossAction)
	sc.Step(`^status identifies the registered running worker with controller absent unknown workload exit and a concrete stop retry$`, w.recoveryLostController)
	sc.Step(`^the observer sees that worker's heartbeat after controller death and before stop$`, w.recoveryHeartbeatAdvanced)
	sc.Step(`^stop removes that exact native worker with confirmed durable completion without claiming successful workload execution$`, func() error {
		if err := w.recoveryCompleted(); err != nil {
			return err
		}
		return w.recoveryOneLaunch()
	})
	sc.Step(`^all checkout edits approvals snapshots and unrelated native resources remain intact$`, w.recoveryPreserved)

	sc.Step(`^two CI recovery checkouts with equal basenames the cached image and independent heartbeat workloads$`, w.recoveryPairFixture)
	sc.Step(`^both product runs start and two independent stop processes target only the first project's session$`, w.recoveryPairAction)
	sc.Step(`^both stops succeed after the first worker's native absence while the second worker continues emitting new heartbeats$`, w.recoveryConcurrentStops)
	sc.Step(`^each project's status lists only its own session with its original fingerprint and cleanup outcome$`, func() error {
		if _, err := w.recoveryEntry(w.recovery.after, "stopped", "absent", "complete"); err != nil {
			return err
		}
		other := w.recovery.second
		s, err := other.recoveryEntry(other.recovery.after, "running", "running", "pending")
		if err != nil {
			return err
		}
		if s.WorkloadExit != nil || s.Controller != "present" || s.StopCandidate {
			return fmt.Errorf("project B lost its live controller or acquired a fabricated workload result")
		}
		return nil
	})
	sc.Step(`^both projects retain their edits approvals and snapshots and no shared image network or volume is removed$`, func() error {
		if err := w.recoveryPreserved(); err != nil {
			return err
		}
		if err := w.recovery.second.recoveryPreserved(); err != nil {
			return err
		}
		return w.recoveryOneLaunch()
	})
}
