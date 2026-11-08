package features

import "github.com/cucumber/godog"

func (w *world) socketSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a CI socket checkout with a live "([^"]*)" host socket at "([^"]*)"$`, func(operation, path string) error {
		return w.socketFixtureTargets("existing", []socketTarget{{Name: "startup", Op: operation, Path: path}})
	})
	sc.Step(`^the product attempts an approved run of the socket checkout$`, w.socketStartupRun)
	sc.Step(`^the socket checkout is refused with exit 1 and run.isolation-unverified before any worker create$`, w.socketStartupRefused)
	sc.Step(`^the startup socket listener remains usable and unchanged$`, func() error {
		return w.sockets.hostControlsFor([]string{"startup"})
	})
	sc.Step(`^the socket refusal preserves existing checkout files approval and unrelated engine resources$`, w.socketStartupPreserved)
	sc.Step(`^a CI socket checkout with the cached image and plans to publish the complete Unix socket operation inventory after launch$`, func() error { return w.socketFixture("late") })
	sc.Step(`^a CI socket checkout with the cached image and ordinary files at every planned host socket path$`, func() error { return w.socketFixture("replace") })
	sc.Step(`^the host publishes those listeners after the product's live barrier and the same worker and its child attempt every socket$`, w.socketRun)
	sc.Step(`^both probe processes read those files and create aliases then the host replaces them with live sockets before either process retries$`, w.socketRun)
	sc.Step(`^every newly published checkout host socket operation is denied in both processes with no worker nonce received by a host listener$`, w.socketDenials)
	sc.Step(`^every replaced checkout host socket operation is denied through absolute relative and tmpfs symlink paths with no worker nonce received by a host listener$`, w.socketDenials)
	sc.Step(`^the socket reports contain both processes' original file reads and every replacement attempt$`, func() error { return w.sockets.completeReports() })
	sc.Step(`^all checkout socket host controls succeed before and after each worker phase$`, func() error { return w.sockets.hostControls() })
	sc.Step(`^all checkout socket worker-private controls succeed in tmpfs$`, func() error {
		if err := w.sockets.completeReports(); err != nil {
			return err
		}
		return socketPrivate(w.sockets.config, w.sockets.reports, true)
	})
	sc.Step(`^the socket probe's regular checkout edit is visible to the host before exit and survives confirmed owned cleanup$`, w.socketEdit)
	sc.Step(`^the product preserves the checkout host listeners protected approval snapshots and unrelated resources$`, w.socketPreserved)
}
