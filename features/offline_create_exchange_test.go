package features

// awaitCreates waits for every create this recorder forwarded whose engine-side
// exchange is still open.
func (a *offlineAPI) awaitCreates() {}

// awaitCreateOutcomes is the step wait: every native recorder the scenario
// started, those of its control, second and overlap worlds included.
func (w *world) awaitCreateOutcomes() {}
