package features

func (w *world) anchorRecoverySteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI anchor recovery checkout with the cached image original and replacement nonce trees and a live identity probe$`, func() error { return w.anchorsFixture("loss") })
	sc.Step(`^run reaches the live probe barrier the fixture replaces the checkout path kills its controller and fresh status and stop processes run$`, w.anchorsLoss)
	sc.Step(`^status identifies the original running session with controller absent anchor present unknown workload exit and the exact stop retry$`, w.anchorsLostStatus)
	sc.Step(`^the live probe after controller death still sees the original checkout inode and edits while replacement and anchor control reads are denied with working controls$`, w.anchorsContinuity)
	sc.Step(`^native inspection retains the same owned volume and unchanged offline boundary until stop confirms worker absence and releases that volume last$`, w.anchorsRelease)
	sc.Step(`^fresh repeated stop and status confirm both absences and durable completion without restarting work or claiming workload success$`, w.anchorsCompleted)
	sc.Step(`^native anchor recovery preserves original edits replacement trees approvals snapshots and unrelated resources before harness cleanup$`, w.anchorsPreserved)

	sc.Step(`^a CI anchor recovery checkout with the cached image a marker workload and a worker-removal-only transient fault$`, func() error { return w.anchorsFixture("retry") })
	sc.Step(`^the anchored workload exits (\d+) and run returns then fresh processes execute its exact stop retry twice after the fault is cleared$`, w.anchorsRetry)
	sc.Step(`^both stops return zero after confirmed native worker and original volume absence and durable anchor completion$`, w.anchorsCompleted)
	sc.Step(`^run preserves observed exit (\d+) returns (\d+) and reports separate run.cleanup-temporary with the registered retry$`, w.anchorsRunResult)
	sc.Step(`^the original volume remains owned through controller exit and is deleted only after worker absence without opening its expired descriptor path$`, w.anchorsRelease)
	sc.Step(`^status retains workload exit (\d+) with anchor absent cleanup complete and retry null without repeated resource mutation$`, w.anchorsExit)

	sc.Step(`^a CI anchor recovery checkout with the cached image and a recorder holding the successful "([^"]*)" response$`, w.anchorsFixture)
	sc.Step(`^run submits that operation the fixture kills only its controller and fresh status and stop processes recover the session$`, w.anchorsUncertain)
	sc.Step(`^recovery verifies the original worker and anchor relationship and confirms worker removal before deleting that same native volume$`, w.anchorsRelease)
	sc.Step(`^status before stop reports the retained anchor and original session without adopting ownership or inventing a workload exit$`, w.anchorsPendingStatus)
	sc.Step(`^any recovered worker ID is durably recorded before mutation and recovery issues no create start exec mount or volume allocation$`, w.anchorsNoRelaunch)
	sc.Step(`^stop and its fresh retry confirm durable completion while preserving any observed workload result and all unrelated native state$`, func() error {
		if err := w.anchorsCompleted(); err != nil {
			return err
		}
		return w.anchorsPreserved()
	})

	sc.Step(`^a CI anchor recovery checkout with the cached image and a volume-delete-only fault plus an independent valid control$`, func() error { return w.anchorsFixture("foreign") })
	sc.Step(`^run reaches its live barrier stop removes the worker and the fixture substitutes a foreign volume before fresh status and stop inspect it$`, w.anchorsForeign)
	sc.Step(`^the first stop has reached owned volume release after confirmed worker absence and the fresh commands report recovery.not-owned with exit 1$`, w.anchorsForeignRefused)
	sc.Step(`^native inspection shows the foreign replacement unchanged with no product volume deletion adoption or completed-session claim$`, w.anchorsForeignPreserved)
	sc.Step(`^the independent native control completes worker and anchor cleanup and both checkouts retain their edits approvals and snapshots$`, func() error {
		if err := w.recovery.second.anchorsCompleted(); err != nil {
			return err
		}
		return w.anchorsPreserved()
	})

	sc.Step(`^two disjoint equal-basename CI anchor checkouts with the cached image independent inode probes and an A anchor-release barrier$`, func() error { return w.anchorsFixture("pair") })
	sc.Step(`^both workloads reach their barriers A loses its controller and two fresh A stops overlap while B continues running$`, w.anchorsPair)
	sc.Step(`^both A stops complete with one native anchor deletion after A worker absence while B produces later heartbeats from its original checkout inode$`, w.anchorsPairCompleted)
	sc.Step(`^B retains its original worker volume ownership and offline boundary and each status lists only its own session and anchor$`, w.anchorsPairStatus)
	sc.Step(`^A cannot stop B's ID and neither repeated stop nor harness observations remove any unrelated native resource$`, w.anchorsCrossRefused)
}
