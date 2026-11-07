package features

import "github.com/cucumber/godog"

// Native checkpoints precede production. The local slice-17 cases still need
// their own red steps and assertion-level evidence.
func (w *world) anchorSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a CI mount-anchor checkout with the cached image original and replacement nonce trees and a reporting probe$`, w.anchorFixture)
	sc.Step(`^the native recorder is armed to pause the submitted worker start before daemon source resolution$`, func() {
		w.anchor.ready, w.anchor.release = make(chan struct{}), make(chan struct{})
	})
	sc.Step(`^run reaches that barrier and the fixture makes "([^"]*)" before forwarding the unchanged start request$`, w.anchorRace)
	sc.Step(`^the worker executes in the original checkout inode and approved subdirectory or run refuses without executing in a substituted checkout$`, w.anchorOriginal)
	sc.Step(`^the independently observed protected anchor was owned before use and the worker has exactly the original nonrecursive checkout host source$`, func() error { return w.anchorCheck("source") })
	sc.Step(`^any started worker cannot read the replacement nonce nested host sentinel or anchor control records$`, func() error { return w.anchorCheck("reads") })
	sc.Step(`^each started worker fails every append rename and unlink attempt against the actual anchor control records while the same operations succeed on worker-local controls$`, func() error { return w.anchorCheck("mutations") })
	sc.Step(`^run confirms worker and anchor absence preserving any original edit and leaving the replacement untouched$`, func() error { return w.anchorCheck("cleanup") })
	sc.Step(`^the no-substitution control executes through its own inode-bound anchor and confirms cleanup$`, w.anchorControl)

	sc.Step(`^a CI mount-anchor checkout with the cached image and a removal-only transient Engine API fault$`, w.anchorRetentionFixture)
	sc.Step(`^the anchored workload is released after checkout replacement and exits (\d+) and the controller returns$`, w.anchorRetentionAction)
	sc.Step(`^the original owned volume and its protected recovery evidence remain after controller exit$`, w.anchorRetained)
	sc.Step(`^native Docker still observes the exact owned worker and no anchor release was attempted$`, w.anchorRetainedWorker)
	sc.Step(`^the observed workload exit is (\d+) and run returns (\d+) with separate "run.cleanup-temporary" guidance and its registered session ID$`, w.anchorRetentionResult)
	sc.Step(`^no completed-session claim is made and checkout edits approval snapshots replacement targets and unrelated resources remain intact$`, w.anchorRetentionPreserved)
	sc.Step(`^the fault-free native control preserves the same workload exit and confirms worker and anchor removal$`, w.anchorRetentionControl)
}
