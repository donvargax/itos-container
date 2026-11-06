package features

import "github.com/cucumber/godog"

// All native access is behind isolationFixture's independent CI guard. These
// bindings leave @wip in place for the coordinator's first native dispatch.
func (w *world) offlineIsolationSteps(sc *godog.ScenarioContext) {
	for _, step := range []struct{ phrase, mode string }{
		{`a CI offline checkout with the cached image and identity capability and setuid probes`, "identity"},
		{`a CI offline checkout with the cached image and a filesystem probe`, "filesystem"},
		{`a CI offline checkout with the cached image and outside host sentinels and socket listeners`, "host"},
		{`a CI offline checkout with the cached image and controlled IPv4 IPv6 TCP UDP gateway private and synthetic metadata listeners`, "network"},
		{`a CI offline checkout with the cached image and controlled system Docker loopback TCP UDP and HTTPS DNS resolvers`, "dns"},
		{`a CI offline checkout with the cached image and a removal-only transient Engine API fault`, "exit"},
	} {
		sc.Step("^"+step.phrase+"$", func() error { return w.isolationFixture(step.mode) })
	}
	sc.Step(`^a CI offline checkout with the cached image and a primary probe exiting (\d+) with a surviving child$`, func(status int) error {
		if err := w.isolationFixture("primary"); err != nil {
			return err
		}
		w.offline.isolation.config.Status = status
		return nil
	})
	sc.Step(`^misleading host identity environment and an unrelated supplementary group are recorded as fixture inputs$`, w.isolationIdentityInputs)
	for _, phrase := range []string{
		`the product runs the offline identity probe as its first executable`,
		`the product runs the offline filesystem probe twice with distinct session nonces`,
		`the product runs the offline host-access probe`,
		`trusted controls and the product's first-instruction and running network probes attempt every fixture route`,
		`trusted controls and the product's first-instruction and running DNS probes query unique fixture names`,
		`the product runs that offline foreground probe`,
	} {
		sc.Step("^"+phrase+"$", w.isolationAction)
	}
	sc.Step(`^the product runs an offline probe that writes a checkout edit and exits (\d+)$`, func(status int) error { w.offline.isolation.config.Status = status; return w.isolationAction() })
	sc.Step(`^the product returns exactly (\d+) after the observed primary exit and confirmed durable owned cleanup$`, func(status int) error { return w.isolationResult(status, status) })
	sc.Step(`^the actual workload exit is (\d+) and the product returns (\d+) with a separate temporary cleanup failure$`, w.isolationResult)
	for _, step := range []struct{ phrase, check string }{
		{`the process and its child have exactly the effective host UID and GID without host supplementary groups`, "identity"},
		{`the runtime has no effective permitted inheritable ambient or bounding capabilities and NoNewPrivs is one`, "capabilities"},
		{`root identity and privileged mount attempts fail and the setuid probe cannot elevate`, "elevation"},
		{`the process reads an effective pids limit of 4096 without any claimed memory or CPU quota`, "pids"},
		{`the product confirms owned cleanup without host chown or unrelated file changes`, "complete"},
		{`both processes observe a read-only image root and fail to modify image files`, "root"},
		{`both processes can write the checkout and their tmpfs HOME and TMPDIR as the host UID GID`, "writes"},
		{`the second process sees the first checkout edit but none of its temporary or home files`, "persistent"},
		{`native inspection and mount observations show only the approved checkout host source and container-owned temporary storage`, "mounts"},
		{`neither worker sees the outside marker from the fixture's nested host mount`, "nested"},
		{`both exact worker IDs and their ephemeral storage are absent after product cleanup with no created persistent volume`, "ephemeral"},
		{`the probe completes its worker-local file and socket controls but cannot read host sentinels or protected approval and ownership records`, "host-files"},
		{`no host home engine socket SSH agent or fixture host socket is reachable from the worker`, "host-sockets"},
		{`PID IPC UTS mount and network namespace identities differ from the host without access to the host process sentinel`, "namespaces"},
		{`host sentinels and unrelated resources are unchanged after confirmed owned cleanup`, "complete"},
		{`every trusted route control succeeds before and after its worker attempt`, "route-controls"},
		{`the worker can reach its own loopback control but no host loopback listener at the same port`, "loopback"},
		{`independent inspection observes NetworkMode none before start and while running with no port publication or extra network attachment`, "network-inspection"},
		{`the product confirms owned cleanup while preserving all unrelated network resources`, "complete"},
		{`each resolver control returns its expected nonce-bearing answer before and after the worker attempt`, "route-controls"},
		{`the worker receives no external answer through system DNS 127\.0\.0\.11 alternate loopback direct TCP UDP or HTTPS DNS`, "route-denials"},
		{`no controlled resolver receives a matching worker query at either execution phase`, "receipts"},
		{`the DNS probes actually execute and the product confirms owned cleanup`, "complete"},
		{`the trailing workload frame is delivered and the descendant no longer runs`, "descendant"},
		{`native Docker confirms the exact worker ID absent while the unrelated same-image worker remains`, "native-absence"},
		{`checkout edits approvals snapshots and unrelated resources remain intact`, "preserved"},
		{`native Docker still observes the exact stopped worker and its protected ownership record before harness cleanup`, "retained"},
		{`the diagnostic carries the safe protected intent reference without the synthetic body marker or a stopped-success claim`, "safe-diagnostic"},
		{`the checkout edit and unrelated resources remain intact`, "preserved"},
	} {
		sc.Step("^"+step.phrase+"$", func() error { return w.isolationCheck(step.check) })
	}
	sc.Step(`^every first-instruction and running worker attempt is denied with no matching listener receipt$`, func() error {
		if err := w.isolationCheck("route-denials"); err != nil {
			return err
		}
		return w.isolationCheck("receipts")
	})
}
