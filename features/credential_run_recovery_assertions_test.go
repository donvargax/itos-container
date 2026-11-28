package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

func (w *world) credentialRunRecoverySteps(sc lifecycleStepRegistrar) {
	sc.Step(`^the run probe waits detached with its own positive control and a separate exec read plan is prepared$`, w.credentialRunDetachedPlans)
	sc.Step(`^the product inspects with JSON doctor then runs the detached probe while the recorder loses the accepted start reply and answers worker removal "([^"]*)" then runs fresh status and any stop it requires$`, w.credentialRunLostStart)
	sc.Step(`^the engine accepted exactly one create and one start of one worker and that start reply never reached the controller$`, w.credentialRunAcceptedStart)
	sc.Step(`^the staged Claude path was absent before the first worker-ending request was forwarded and an exec in the running worker then read the exact source hash from the fixed Claude path as the non-root host UID$`, w.credentialRunReadAfterUnlink)
	sc.Step(`^the workload exited 23 by itself and run returned 23 without a repeated start a replacement worker or a synthesized exit$`, w.credentialRunObservedExit)
	sc.Step(`^run reports "([^"]*)" cleanup for its registered session$`, w.credentialRunCleanupReport)
	sc.Step(`^fresh status reports that session with controller absent workload exit 23 credential staging "([^"]*)" and cleanup "([^"]*)" in plain and JSON output$`, w.credentialRunLostStartStatus)
	sc.Step(`^the final status after any required fresh stop reports complete cleanup and no worker staging or other owned resource remains$`, w.credentialRunLostStartFinal)
	sc.Step(`^the product inspects with JSON doctor then runs the detached probe until the recorder holds the engine's reply "([^"]*)" and the harness kills only the controller with SIGKILL$`, w.credentialRunKilledAtBarrier)
	sc.Step(`^after reaping the controller the fixture unlinks the policy then the product runs fresh status then fresh stop then fresh status again$`, w.credentialRunFreshRecovery)
	sc.Step(`^the controller was killed while the engine had accepted the held "([^"]*)" call and the worker "([^"]*)"$`, w.credentialRunBarrierObserved)
	sc.Step(`^fresh status before stop reports the original session with controller absent cleanup pending no workload exit credential staging "([^"]*)" and the exact stop retry in plain and JSON output$`, w.credentialRunKilledStatus)
	sc.Step(`^fresh stop removes the original worker and its host staging directory and confirms complete cleanup$`, w.credentialRunStopRemoved)
	sc.Step(`^fresh status after stop reports complete cleanup credential staging "([^"]*)" and no workload exit or stop retry$`, w.credentialRunKilledFinal)
	sc.Step(`^no start restart or create followed the kill and status and stop read neither the source nor the active approval without a current policy$`, w.credentialRunOnlyProtectedRecovery)
	sc.Step(`^no provisioning surface of the recovery contains the raw marker or any base64-offset encoding$`, w.credentialRunRecoverySurfaces)
	sc.Step(`^the source approval and unrelated resources are unchanged and the checkout differs only by the scenario's journaled policy edits$`, w.credentialRunRecoveryUnchanged)
}

func credentialCreateExchange(exchange credentialRunExchange) bool {
	return exchange.Method == "POST" && credentialRunAPIPath(exchange.Path) == "/containers/create"
}

func (w *world) credentialRunAcceptedStart() error {
	f := w.credentialRun
	r := f.projects[0].runs[0]
	log := f.journal.faultLog()
	if log.Creates != 1 || log.Starts != 1 || log.StartAccepted.IsZero() || !log.StartReplyWithheld || r.session == "" {
		return credentialRunFailure(r, "engine did not record exactly one accepted create and unacknowledged start for the registered worker")
	}
	creates := 0
	for _, exchange := range f.journal.snapshot() {
		if credentialCreateExchange(exchange) {
			creates++
		}
	}
	if creates != 1 {
		return errors.New("controller requested an additional worker create")
	}
	return nil
}

func (w *world) credentialRunReadAfterUnlink() error {
	f := w.credentialRun
	p := f.projects[0]
	rec := &f.recovery
	log := f.journal.faultLog()
	probe := rec.probe
	if log.Ending == nil || log.Ending.Expired || !log.Ending.Released || !rec.absentAt.After(log.StartAccepted) ||
		probe.started.Before(rec.absentAt) || probe.finished.IsZero() || !probe.finished.Before(log.Ending.Forwarded) ||
		probe.exit != 0 || !probe.first.hasFile(credentialRunHash(p.originalSource)) || !credentialCannotOpen(probe.first, p.source, p.sibling) {
		return errors.New("independent exec did not read the approved copy between staging unlink and the first forwarded ending request")
	}
	return nil
}

func (w *world) credentialRunObservedExit() error {
	f := w.credentialRun
	r := f.projects[0].runs[0]
	log := f.journal.faultLog()
	if f.recovery.exitedAt.IsZero() || log.Ending == nil || !f.recovery.exitedAt.Before(log.Ending.Forwarded) ||
		r.exit != 23 || log.Starts != 1 || log.Creates != 1 || bytes.Contains(r.diagnostic.bytes(), []byte("run.workload-exit-unknown")) {
		return credentialRunFailure(r, "run did not preserve the independently observed workload exit 23")
	}
	return nil
}

func credentialRunRetry(id string) string {
	return "Use the same effective project: itos container stop " + id
}

func (w *world) credentialRunCleanupReport(cleanup string) error {
	f := w.credentialRun
	r := f.projects[0].runs[0]
	diagnostic := string(r.diagnostic.bytes())
	temporary := strings.Contains(diagnostic, "run.cleanup-temporary")
	ownership := strings.Contains(diagnostic, "run.cleanup-ownership")
	switch cleanup {
	case "complete":
		if r.session != "" && !temporary && !ownership {
			return nil
		}
	case "incomplete":
		if r.session != "" && f.journal.faultLog().RefusedRemovals > 0 && temporary && strings.Contains(diagnostic, credentialRunRetry(r.session)) {
			return nil
		}
	}
	return fmt.Errorf("run did not report the expected %q cleanup for its registered session", cleanup)
}

func credentialRunStatusSession(result *homeNativeResult, id string) map[string]any {
	if result == nil || result.err != nil || result.exit != 0 || id == "" {
		return nil
	}
	report := credentialRunObject(result.out)
	problems, ok := report["problems"].([]any)
	sessions, _ := report["sessions"].([]any)
	if report["schema"] != float64(1) || report["ok"] != true || !ok || len(problems) != 0 || len(sessions) != 1 {
		return nil
	}
	session := object(sessions[0])
	if session["id"] != id {
		return nil
	}
	return session
}

func credentialRunPlainSummary(result *homeNativeResult, id, project, fingerprint string) (int, []string, []string) {
	if result == nil || result.err != nil || result.exit != 0 {
		return -1, nil, nil
	}
	lines := strings.Split(string(result.out), "\n")
	index := -1
	var summary []string
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 18 || fields[0] != "session" || fields[1] != id {
			continue
		}
		if index >= 0 ||
			!slices.Equal(fields[2:7], []string{"project", project, "fingerprint", fingerprint, "state"}) ||
			fields[8] != "resource" || fields[10] != "controller" || fields[12] != "cleanup" ||
			fields[14] != "workload_exit" || fields[16] != "stop_candidate" {
			return -1, nil, nil
		}
		index, summary = i, fields
	}
	return index, summary, lines
}

func credentialRunCheckStatus(pair credentialRunStatusPair, p *credentialRunProject, r *credentialRunProcess, cleanup, staging string, exit any) error {
	pending := cleanup == "pending"
	var retry any
	if pending {
		retry = []any{"itos", "container", "stop", r.session}
	}
	received := credentialRunStatusSession(pair.json, r.session)
	for key, want := range map[string]any{
		"project": p.w.approval.project, "fingerprint": p.w.approval.fingerprint,
		"controller": "absent", "cleanup": cleanup, "credential_staging": staging,
		"workload_exit": exit, "stop_candidate": pending, "retry": retry,
	} {
		if got, present := received[key]; !present || !reflect.DeepEqual(got, want) {
			return fmt.Errorf("JSON status did not report the expected %s for the registered session", key)
		}
	}
	index, summary, lines := credentialRunPlainSummary(pair.plain, r.session, p.w.approval.project, p.w.approval.fingerprint)
	exitText := "unknown"
	if code, ok := exit.(float64); ok {
		exitText = strconv.Itoa(int(code))
	}
	if index < 0 || summary[11] != "absent" || summary[13] != cleanup || summary[15] != exitText || summary[17] != strconv.FormatBool(pending) {
		return errors.New("plain status summary did not match JSON status")
	}
	stagingIndex, retryIndex := -1, -1
	stagingPrefix := "session " + r.session + " credential staging: "
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, stagingPrefix):
			if stagingIndex >= 0 || line != stagingPrefix+staging {
				return errors.New("plain status credential staging did not match JSON status")
			}
			stagingIndex = i
		case strings.HasPrefix(line, "Use the same effective project:"):
			if retryIndex >= 0 || line != credentialRunRetry(r.session) {
				return errors.New("plain status did not report the exact stop retry")
			}
			retryIndex = i
		}
	}
	if stagingIndex <= index || pending && retryIndex <= stagingIndex || !pending && retryIndex >= 0 {
		return errors.New("plain status staging or retry line was missing or out of order")
	}
	return nil
}

func (w *world) credentialRunLostStartStatus(staging, cleanup string) error {
	f := w.credentialRun
	p := f.projects[0]
	return credentialRunCheckStatus(f.recovery.initial, p, p.runs[0], cleanup, staging, float64(23))
}

func (w *world) credentialRunLostStartFinal() error {
	f := w.credentialRun
	p := f.projects[0]
	r := p.runs[0]
	if credentialRunStatusSession(f.recovery.initial.json, r.session)["stop_candidate"] == true {
		if err := w.credentialRunStopRemoved(); err != nil {
			return err
		}
	}
	if !f.recovery.allRemoved() {
		return errors.New("final inspection still showed owned worker or staging resources")
	}
	return credentialRunCheckStatus(f.recovery.final, p, r, "complete", "absent", float64(23))
}

func (w *world) credentialRunBarrierObserved(barrier, workerState string) error {
	f := w.credentialRun
	p := f.projects[0]
	r := p.runs[0]
	rec := &f.recovery
	log := f.journal.faultLog()
	if !credentialRunWasKilled(r) || log.Barrier != barrier || log.BarrierAt.IsZero() || !log.BarrierAt.Before(rec.killAt) ||
		log.BarrierEnded.Before(rec.killAt) || log.Creates != 1 || rec.workerState == nil || r.session == "" {
		return credentialRunFailure(r, "controller kill at the accepted engine reply was not observed for the registered worker")
	}
	switch barrier {
	case credentialCreateBarrier:
		if workerState == "was created and never started" && rec.workerState.Status == "created" &&
			!rec.workerState.Running && credentialRunZeroStart(rec.workerState.StartedAt) && log.Starts == 0 {
			return nil
		}
	case credentialStartBarrier:
		if workerState == "ran and an exec read the exact source hash from the still staged copy" && rec.workerState.Running &&
			log.Starts == 1 && credentialStageFile(r.staging) && rec.stagingAtKill[filepath.Base(filepath.Dir(r.staging))] &&
			rec.probe.exit == 0 && rec.probe.first.hasFile(credentialRunHash(p.originalSource)) {
			return nil
		}
	}
	return errors.New("worker state or staged-copy observation did not match the held barrier")
}

func (w *world) credentialRunKilledStatus(staging string) error {
	f := w.credentialRun
	p := f.projects[0]
	return credentialRunCheckStatus(f.recovery.initial, p, p.runs[0], "pending", staging, nil)
}

func (w *world) credentialRunStopRemoved() error {
	f := w.credentialRun
	p := f.projects[0]
	r := p.runs[0]
	_, summary, _ := credentialRunPlainSummary(f.recovery.stop, r.session, p.w.approval.project, p.w.approval.fingerprint)
	if len(summary) == 0 || summary[13] != "complete" || !f.recovery.allRemoved() {
		return errors.New("fresh stop did not confirm removal of the original worker, staging and other owned resources")
	}
	return nil
}

func (w *world) credentialRunKilledFinal(staging string) error {
	f := w.credentialRun
	p := f.projects[0]
	return credentialRunCheckStatus(f.recovery.final, p, p.runs[0], "complete", staging, nil)
}

func (w *world) credentialRunOnlyProtectedRecovery() error {
	f := w.credentialRun
	rec := &f.recovery
	workerPath := "/containers/" + f.journal.faultLog().Worker
	for _, exchange := range rec.afterKill {
		if exchange.Method != "POST" {
			continue
		}
		action, originalWorker := strings.CutPrefix(credentialRunAPIPath(exchange.Path), workerPath)
		if credentialCreateExchange(exchange) || originalWorker && (action == "/start" || action == "/restart" || action == "/unpause") {
			return errors.New("controller recovery attempted to create or relaunch a worker")
		}
	}
	if !rec.policyAbsent || !rec.noInputAccess {
		return errors.New("recovery did not keep policy absent or touched an existing source or approval")
	}
	return nil
}

func (w *world) credentialRunRecoverySurfaces() error {
	f := w.credentialRun
	p := f.projects[0]
	r := p.runs[0]
	rec := &f.recovery
	if !f.controlled || len(f.journal.snapshot()) == 0 || !json.Valid(r.workerRaw) || !rec.collected {
		return errors.New("marker controls, engine journal, run or worker capture is missing")
	}
	if err := f.journal.scan(f.scan); err != nil {
		return err
	}
	if p.checkoutEditRejected {
		return errors.New("recovery policy edit failed its observer bracket")
	}
	if err := p.observer.judge(); err != nil {
		if leak := f.scan("checkout observer diagnostic", []byte(err.Error())); leak != nil {
			return leak
		}
		return err
	}
	edits := p.observer.mutations
	if rec.killAt.IsZero() && len(edits) != 0 || !rec.killAt.IsZero() && (len(edits) != 1 || edits[0].Op != "unlink") {
		return errors.New("recovery checkout did not record the expected policy edits")
	}
	for name, result := range map[string]*homeNativeResult{
		"JSON doctor": p.doctor, "initial plain status": rec.initial.plain, "initial JSON status": rec.initial.json,
		"final plain status": rec.final.plain, "final JSON status": rec.final.json,
	} {
		if err := f.scanTranscript(name, result); err != nil {
			return err
		}
	}
	if !rec.killAt.IsZero() || credentialRunStatusSession(rec.initial.json, r.session)["stop_candidate"] == true {
		if err := f.scanTranscript("fresh stop", rec.stop); err != nil {
			return err
		}
	}
	for name, data := range map[string][]byte{
		"run stdout": r.out.bytes(), "run stderr": r.diagnostic.bytes(), "worker inspect": r.workerRaw,
		"exec stdout": rec.probe.out, "exec stderr": rec.probe.diagnostic,
	} {
		if err := f.scan(name, data); err != nil {
			return err
		}
	}
	if f.journal.faultLog().Barrier != credentialCreateBarrier &&
		(rec.probe.finished.IsZero() || rec.probe.first.Nonce == "" || rec.probe.final.Nonce == "") {
		return errors.New("recovery exec capture or reports are missing")
	}
	return f.scanCredentialRunTrees(p.w.doctor.checkout, f.state, f.fixedState())
}

func (w *world) credentialRunRecoveryUnchanged() error {
	f := w.credentialRun
	p := f.projects[0]
	r := p.runs[0]
	checkout, err := doctorTree(p.w.doctor.checkout)
	if err != nil {
		return err
	}
	engine, err := p.w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	expected := maps.Clone(p.checkoutBefore)
	if !f.recovery.killAt.IsZero() {
		delete(expected, filepath.Join(p.w.doctor.checkout, "itos-container.yaml"))
	}
	approval := credentialRunObject(r.approvalBefore)
	if !maps.Equal(expected, checkout) || !maps.Equal(f.originalResources, engine) ||
		!credentialOriginalFile(p.source, p.originalSource, p.sourceInfo) || !credentialOriginalFile(p.sibling, p.originalSibling, p.siblingInfo) ||
		!bytes.Equal(r.approvalBefore, r.approvalAfter) || approval["fingerprint"] != p.w.approval.fingerprint || approval["generation"] != p.w.approval.generation {
		return errors.New("checkout, source, sibling, approval or unrelated resources changed during recovery")
	}
	return nil
}
