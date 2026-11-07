package features

import "errors"

func (w *world) lifecycleLossSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI lifecycle checkout with continuous boundary probes and working external route and host-access controls$`, w.lifecycleLossFixture)
	sc.Step(`^run launches the probe and the harness kills only the controller with SIGKILL then requests a fresh probe round before status and stop$`, w.lifecycleLossAction)
	sc.Step(`^status identifies the lost-controller session as a stop candidate with unknown workload outcome and its exact retry removes only that owned worker$`, func() error {
		return errors.Join(w.loss.err, w.recoveryLostController(), w.recoveryCompleted(), w.recoveryOneLaunch())
	})
	sc.Step(`^the same worker reports a fresh post-loss round under the unchanged offline filesystem identity and network boundary while remaining alive until stop$`, w.lossFreshBoundary)
	sc.Step(`^all external route and resolver controls succeed while every worker attempt is denied with no matching listener receipt$`, w.lossNetwork)
	sc.Step(`^host sentinels sockets approval and recovery state remain inaccessible while worker-local controls and checkout writes succeed$`, w.lossHost)
	sc.Step(`^stop revalidates the full ID and confirms durable absence without changing the original unknown workload history solely because removal succeeded$`, w.lossDurableStop)
	sc.Step(`^no automatic restart replacement worker new permission or synthetic workload success is reported and checkout edits survive$`, func() error {
		return errors.Join(w.lossPreserved(), w.recoveryOneLaunch(), w.lossDurableStop())
	})
}
