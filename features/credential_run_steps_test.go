package features

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"
)

func (w *world) credentialRunSteps(sc lifecycleStepRegistrar) {
	w.credentialRunRecoverySteps(sc)
	sc.Step(`^a CI Claude checkout with a private synthetic source and matching "([^"]*)" approval$`, w.credentialRunSingle)
	sc.Step(`^the cooperative credential probe and every marker detector have independent positive controls$`, w.credentialRunControls)
	sc.Step(`^the product inspects with doctor and matching terminal approve then runs the credential probe through active status and workload exit (\d+)$`, w.credentialRunSingleWhen)
	sc.Step(`^the native fixture verification passed and JSON doctor reported every readiness check and (?:the|each) Claude source ready$`, w.credentialRunReady)
	sc.Step(`^the worker reports the exact source hash including final newlines from the fixed Claude path as the non-root host UID$`, w.credentialRunFirstHash)
	sc.Step(`^native inspection and worker probes show one read-only mode-0400 regular Claude file and all existing hardening$`, w.credentialRunHardening)
	sc.Step(`^write attempts fail and the original host source its sibling and host staging directory cannot be opened from the worker$`, w.credentialRunIsolation)
	sc.Step(`^no controller-set environment or argv contains a token or automatically exports CLAUDE_CODE_OAUTH_TOKEN$`, w.credentialRunEnvironment)
	sc.Step(`^its network evidence is "([^"]*)"$`, w.credentialRunNetwork)
	sc.Step(`^every specified cooperative provisioning surface contains neither the raw marker nor any base64-offset encoding$`, w.credentialRunSurfaces)
	sc.Step(`^staged paths are absent after acknowledged startup while the worker can still read the copy$`, w.credentialRunUnlinked)
	sc.Step(`^run returns the observed workload exit (\d+) after confirmed removal of the worker staging and all other owned resources$`, w.credentialRunExit)
	sc.Step(`^the checkout source approval and unrelated resources are unchanged$`, w.credentialRunUnchanged)

	sc.Step(`^a second distinct synthetic source value is reserved for rotation by "([^"]*)"$`, w.credentialRunReserve)
	sc.Step(`^the product inspects with JSON doctor then starts a waiting hash probe then the fixture rotates the source and releases a second read$`, w.credentialRunRotate)
	sc.Step(`^both reads in that worker report the original hash and the source reports only the rotated value$`, w.credentialRunOriginalCopy)
	sc.Step(`^that worker exits and the product runs a fresh hash probe without another approval$`, w.credentialRunRotatedRun)
	sc.Step(`^the fresh worker reports the rotated hash through a newly staged Claude file$`, w.credentialRunRotatedCopy)
	sc.Step(`^both runs keep the original approval fingerprint and generation without a consent prompt$`, w.credentialRunApprovals)
	sc.Step(`^both runs confirm complete owned cleanup and no cooperative provisioning leak$`, w.credentialRunAllClean)

	sc.Step(`^two independent CI Claude projects with distinct synthetic sources and separately matching approvals$`, w.credentialRunTwoProjects)
	sc.Step(`^a third independent approved version three project requests no credentials$`, w.credentialRunThirdProject)
	sc.Step(`^the product inspects every project with JSON doctor then runs their probes concurrently and stops the first project while the other probes remain active$`, w.credentialRunParallel)
	sc.Step(`^each credential worker reports only its own source hash and cannot open the other project's source or staged path$`, w.credentialRunSeparateGrants)
	sc.Step(`^the no-grant worker executes its probe and cannot open either token or the fixed Claude path$`, w.credentialRunThirdIsolated)
	sc.Step(`^stopping the first project removes only its resources while the second still reads its original copy$`, w.credentialRunStoppedFirst)
	sc.Step(`^the remaining probes finish and the fixture removes the first project's grant and independently seeds its matching no-grant approval$`, w.credentialRunRemoveGrant)
	sc.Step(`^the product runs the no-grant token probe in the first project's same checkout and project identity$`, w.credentialRunLater)
	sc.Step(`^that later run has a fresh session and its probe reports a nonce-bearing denial from the fixed Claude path$`, w.credentialRunLaterSession)
	sc.Step(`^that later run executes without source reads staging credential mounts or residual token access$`, w.credentialRunLaterIsolated)
	sc.Step(`^every run confirms its owned cleanup without modifying any original source or leaking a marker through provisioning$`, w.credentialRunAllClean)
}

func (w *world) credentialRunSingle(network string) error {
	f, err := w.openCredentialRun()
	if err != nil {
		return err
	}
	p, err := w.credentialRunProject(network, true)
	if err != nil {
		return err
	}
	var child json.RawMessage
	if network == "allowlist" {
		child, err = f.setupEgress(p)
		if err != nil {
			return err
		}
	}
	p.sibling = filepath.Join(p.sourceDir, "claude-token-sibling")
	p.originalSibling, err = f.writeMarker(p.sibling, 0600)
	if err != nil {
		return err
	}
	p.siblingInfo, err = os.Lstat(p.sibling)
	if err != nil {
		return err
	}
	if err := p.plan([]string{p.source, p.sibling}, child); err != nil {
		return err
	}
	return f.ready()
}

func (w *world) credentialRunControls() error {
	f := w.credentialRun
	if err := f.positiveProbe(f.projects[0]); err != nil {
		return err
	}
	return f.positiveDetectors()
}

func (w *world) credentialRunSingleWhen(exit int) error {
	f := w.credentialRun
	p := f.projects[0]
	if err := f.arm(); err != nil {
		return err
	}
	if err := p.inspectBeforeRun(); err != nil {
		return err
	}
	r, err := p.start(1)
	if err != nil {
		return err
	}
	if err := f.connectEgress(p, r); err != nil {
		return err
	}
	if err := f.awaitAndObserve(p, r); err != nil {
		return err
	}
	if r.first.Nonce != "" {
		status := p.w.homeNativeInvoke("status")
		jsonStatus := p.w.homeNativeInvoke("status", "--json")
		r.status, r.jsonStatus = &status, &jsonStatus
		if err := errors.Join(status.err, jsonStatus.err); err != nil {
			return err
		}
		r.stagingAbsentLive, err = f.waitForStartupUnlink(r)
		if err != nil {
			return err
		}
		if err := f.readAgain(r); err != nil {
			return err
		}
	}
	return f.finish(p, r, &exit)
}

func (f *credentialRunFixture) awaitAndObserve(p *credentialRunProject, r *credentialRunProcess) error {
	var err error
	r.first, err = r.await("first")
	if err != nil || r.first.Nonce == "" {
		return err
	}
	return f.observe(p, r)
}

func (f *credentialRunFixture) readAgain(r *credentialRunProcess) error {
	if err := r.send("read"); err != nil {
		return err
	}
	var err error
	r.read, err = r.await("read")
	return err
}

func (w *world) credentialRunReserve(rotation string) error {
	f := w.credentialRun
	f.rotation = rotation
	var err error
	f.rotated, err = f.marker()
	if err != nil {
		return err
	}
	if err := f.projects[0].plan(nil, nil); err != nil {
		return err
	}
	return w.credentialRunControls()
}

func (w *world) credentialRunRotate() error {
	f := w.credentialRun
	p := f.projects[0]
	if err := f.arm(); err != nil {
		return err
	}
	if err := p.runDoctor(); err != nil {
		return err
	}
	r, err := p.start(1)
	if err != nil {
		return err
	}
	if err := f.awaitAndObserve(p, r); err != nil {
		return err
	}
	if r.first.Nonce == "" {
		return f.finish(p, r, nil)
	}
	if err := f.rotate(p); err != nil {
		return err
	}
	return f.readAgain(r)
}

func (f *credentialRunFixture) rotate(p *credentialRunProject) error {
	switch f.rotation {
	case "rewriting the private file in place":
		file, err := os.OpenFile(p.source, os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		_, err = file.Write(f.rotated)
		if err := errors.Join(err, file.Close()); err != nil {
			return err
		}
	case "replacing it with a valid new inode":
		stage := filepath.Join(p.sourceDir, "claude-token.next")
		if err := os.WriteFile(stage, f.rotated, 0600); err != nil {
			return err
		}
		if err := os.Rename(stage, p.source); err != nil {
			return err
		}
	default:
		return errors.New("unknown fixture rotation")
	}
	body, err := os.ReadFile(p.source)
	if err != nil {
		return err
	}
	info, err := os.Lstat(p.source)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0600 || os.SameFile(p.sourceInfo, info) != (f.rotation == "rewriting the private file in place") {
		return errors.New("fixture did not rotate the source by the requested operation")
	}
	f.rotatedInfo = info
	f.sourceAfterRotation = credentialRunHash(body)
	return nil
}

func (w *world) credentialRunRotatedRun() error {
	f := w.credentialRun
	if err := f.arm(); err != nil {
		return err
	}
	p := f.projects[0]
	exit := 0
	if err := f.finish(p, p.runs[0], &exit); err != nil {
		return err
	}
	r, err := p.start(2)
	if err != nil {
		return err
	}
	if err := f.awaitAndObserve(p, r); err != nil {
		return err
	}
	return f.finish(p, r, &exit)
}

func (w *world) credentialRunTwoProjects() error {
	if _, err := w.openCredentialRun(); err != nil {
		return err
	}
	for range 2 {
		if _, err := w.credentialRunProject("offline", true); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) credentialRunThirdProject() error {
	f := w.credentialRun
	p, err := w.credentialRunProject("offline", false)
	if err != nil {
		return err
	}
	first, second := f.projects[0], f.projects[1]
	both := []string{first.source, second.source}
	for i, project := range f.projects {
		forbidden := both
		if i < 2 {
			forbidden = []string{both[1-i]}
		}
		if err := project.plan(forbidden, nil); err != nil {
			return err
		}
	}
	if err := first.plan(both, nil); err != nil {
		return err
	}
	if err := f.positiveProbe(p); err != nil {
		return err
	}
	if err := f.positiveDetectors(); err != nil {
		return err
	}
	return f.ready()
}

func (w *world) credentialRunParallel() error {
	f := w.credentialRun
	if err := f.arm(); err != nil {
		return err
	}
	for _, p := range f.projects {
		if err := p.runDoctor(); err != nil {
			return err
		}
	}
	for _, p := range f.projects {
		if _, err := p.start(1); err != nil {
			return err
		}
	}
	allLive := true
	for _, p := range f.projects {
		if err := f.awaitAndObserve(p, p.runs[0]); err != nil {
			return err
		}
		allLive = allLive && p.runs[0].first.Nonce != ""
	}
	for _, p := range f.projects {
		if p.runs[0].first.Nonce == "" {
			if err := f.finish(p, p.runs[0], nil); err != nil {
				return err
			}
		}
	}
	if !allLive {
		return nil
	}
	first, second := f.projects[0], f.projects[1]
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	peers := map[*credentialRunProject][]string{}
	for _, p := range f.projects[1:] {
		owned, err := p.engine(f.cli).owned(ctx)
		if err != nil {
			return err
		}
		slices.Sort(owned)
		peers[p] = owned
	}
	if session := first.runs[0].session; session != "" {
		stop := first.w.homeNativeInvoke("stop", session)
		first.stop = &stop
		if stop.err != nil {
			return stop.err
		}
		if stop.exit == 0 {
			if err := f.finish(first, first.runs[0], nil); err != nil {
				return err
			}
		}
	}
	if err := f.readAgain(second.runs[0]); err != nil {
		return err
	}
	// Compare the same live resource IDs across stop, including the third worker.
	for _, p := range f.projects[1:] {
		r := p.runs[0]
		if r.observed == nil || r.observed.worker == nil {
			continue
		}
		owned, err := p.engine(f.cli).owned(ctx)
		if err != nil {
			return err
		}
		slices.Sort(owned)
		if !slices.Equal(peers[p], owned) {
			continue
		}
		var after json.RawMessage
		if err := p.w.doctor.real.read("/containers/"+r.observed.worker.ID+"/json", &after, 200); err != nil {
			return err
		}
		r.peerSurvived = object(credentialRunObject(after)["State"])["Running"] == true
	}
	return nil
}

func (w *world) credentialRunRemoveGrant() error {
	f := w.credentialRun
	if err := f.arm(); err != nil {
		return err
	}
	exit := 0
	for _, p := range f.projects {
		if err := f.finish(p, p.runs[0], &exit); err != nil {
			return err
		}
	}
	first := f.projects[0]
	projectID := first.w.approval.project
	first.w.credentialApproval.grants = nil
	if err := first.observer.editPolicy("rename", first.policy(), nil); err != nil {
		var rejection credentialRunBracketRejection
		if !errors.As(err, &rejection) {
			return err
		}
		first.checkoutEditRejected = true
	}
	if err := first.seed("5"); err != nil {
		return err
	}
	if first.w.approval.project != projectID {
		return errors.New("rewriting policy changed the fixture project identity")
	}
	return nil
}

func (w *world) credentialRunLater() error {
	f := w.credentialRun
	if err := f.arm(); err != nil {
		return err
	}
	p := f.projects[0]
	p.w.credentials.paths["peer"] = f.projects[1].source
	finishWatch, err := p.w.credentialApprovalWatch()
	if err != nil {
		return err
	}
	defer func() { finishWatch() }()
	staging, err := newCredentialRunStagingObserver()
	if err != nil {
		return err
	}
	defer func() { _ = staging.close() }()
	r, err := p.start(2)
	if err != nil {
		return err
	}
	f.later = r
	if err := f.awaitAndObserve(p, r); err != nil {
		return err
	}
	exit := 0
	if err := f.finish(p, r, &exit); err != nil {
		return err
	}
	// The watcher intentionally spans only the later process.
	finishWatch()
	finishWatch = func() {}
	r.noSourceAccess = p.w.credentialApproval.accessError == nil
	events, err := staging.events()
	if err != nil {
		return err
	}
	r.noStaging = true
	for _, event := range events {
		if event.kind == credentialOverflow || credentialStageName(filepath.Base(event.path)) {
			r.noStaging = false
		}
	}
	return nil
}
