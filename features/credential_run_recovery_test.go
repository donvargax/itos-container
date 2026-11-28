package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type credentialRunStatusPair struct {
	plain, json *homeNativeResult
}

type credentialRunRecovery struct {
	initial, final                           credentialRunStatusPair
	stop                                     *homeNativeResult
	probe                                    credentialRunExec
	killAt, absentAt, exitedAt               time.Time
	workerState                              *container.State
	stagingAtKill                            map[string]bool
	owned                                    []string
	shm                                      map[string]bool
	workerAbsent, directoryAbsent, collected bool
	policyAbsent, noInputAccess              bool
	afterKill                                []credentialRunExchange
}

func (f *credentialRunFixture) startRecovery(faults credentialRunFaults) (*credentialRunProject, *credentialRunProcess, error) {
	p := f.projects[0]
	if err := f.arm(); err != nil {
		return p, nil, err
	}
	if err := p.runDoctor(); err != nil {
		return p, nil, err
	}
	f.journal.arm(faults)
	r, err := p.start(2)
	return p, r, err
}

func credentialRunAwaitWorker(r *credentialRunProcess, notification <-chan string) (string, error) {
	timer := time.NewTimer(engineReadinessAllowance)
	defer timer.Stop()
	select {
	case worker := <-notification:
		return worker, nil
	case <-r.done:
		select {
		case worker := <-notification:
			return worker, r.waitErr
		default:
			return "", r.waitErr
		}
	case <-timer.C:
		return "", errors.New("credential recorder readiness deadline expired")
	}
}

func (f *credentialRunFixture) observeRecoveryWorker(ctx context.Context, p *credentialRunProject, r *credentialRunProcess, id string) (*container.State, error) {
	worker, err := f.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return nil, err
	}
	if worker.Container.ID != id || worker.Container.State == nil || worker.Container.Config == nil ||
		worker.Container.Config.Labels["io.itos-container.project"] != p.w.approval.project {
		return nil, errors.New("credential recovery inspect did not identify the recorder's worker")
	}
	r.workerRaw = worker.Raw
	r.session, _, _ = p.w.homeNativeRegistration(credentialRunObject(r.workerRaw))
	for _, mount := range worker.Container.Mounts {
		if mount.Destination == credentialRunTarget {
			r.staging = mount.Source
		}
	}
	if r.staging == "" && worker.Container.HostConfig != nil {
		for _, mount := range worker.Container.HostConfig.Mounts {
			if mount.Target == credentialRunTarget {
				r.staging = mount.Source
			}
		}
	}
	return worker.Container.State, nil
}

func (w *world) credentialRunLostStart(removal string) error {
	f := w.credentialRun
	defer f.journal.clear()
	p, r, err := f.startRecovery(credentialRunFaults{
		LoseStart: true, FailRemoval: removal == "with 503 until run exits",
		StagedAbsent: func() (bool, error) {
			entries, err := f.newShmEntries()
			return len(entries) == 0, err
		},
	})
	if err != nil {
		return err
	}
	worker, err := credentialRunAwaitWorker(r, f.journal.accepted())
	if err != nil || worker == "" {
		return err
	}
	if err := f.credentialRunReconcile(p, r, worker); err != nil {
		return err
	}
	<-r.done
	f.journal.clear()
	if err := errors.Join(r.waitErr, f.journal.faultObservationError()); err != nil {
		return err
	}
	return f.recoveryCommands(p, r, worker, false)
}

func (f *credentialRunFixture) credentialRunReconcile(p *credentialRunProject, r *credentialRunProcess, worker string) error {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	state, err := f.observeRecoveryWorker(ctx, p, r, worker)
	if err != nil {
		return err
	}
	if !credentialStageFile(r.staging) || !state.Running {
		return nil
	}
	execID, err := f.prepareCredentialExec(ctx, p, worker, "/workspace/credrun-probe", "/workspace/"+credentialPlanName(3))
	if err != nil {
		return err
	}
	absent, err := f.waitRecoveryUnlink(r)
	if err != nil || !absent {
		return err
	}

	started := f.recovery.absentAt
	ctxRead, cancelRead := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancelRead()
	// The ending call can arrive during the exec. Its independently recorded
	// arrival shortens the deadline even when the unlink won that race.
	go func() {
		select {
		case <-f.journal.faults.endingArrived:
			ending := f.journal.faultLog().Ending
			timer := time.NewTimer(time.Until(ending.Arrived.Add(f.journal.faults.hold)))
			defer timer.Stop()
			select {
			case <-timer.C:
				cancelRead()
			case <-ctxRead.Done():
			}
		case <-ctxRead.Done():
		}
	}()
	if err := f.readAndExitCredentialWorker(ctxRead, p, worker, execID); err != nil {
		return fmt.Errorf("lost-start exec and exit after %s: %w", time.Since(started).Round(time.Millisecond), err)
	}
	return nil
}

func (f *credentialRunFixture) readAndExitCredentialWorker(ctx context.Context, p *credentialRunProject, worker, execID string) error {
	if ending := f.journal.faultLog().Ending; ending != nil && !ending.Forwarded.IsZero() {
		if ending.Expired && (ending.StagedAbsentAtArrival || f.recovery.absentAt.Before(ending.Forwarded)) {
			return errors.New("lost-start read missed the ending hold")
		}
		return nil
	}
	state, err := f.credentialWorkerState(ctx, worker)
	if err != nil {
		if errdefs.IsNotFound(err) && f.endingForwarded() {
			return nil
		}
		return err
	}
	if !state.Running && f.endingForwarded() {
		return nil
	}
	if !state.Running {
		return errors.New("credential worker stopped before the independent exec")
	}
	if f.recovery.probe, err = f.readCredentialExec(ctx, execID, p.plans[2]); err != nil {
		return err
	}
	if err := f.signalCredentialExit(ctx, worker); err != nil {
		return err
	}
	f.recovery.exitedAt = time.Now()
	ending := f.journal.faultLog().Ending
	if ending != nil && !f.recovery.exitedAt.Before(ending.Arrived.Add(f.journal.faults.hold)) {
		return errors.New("lost-start exec and exit exceeded the ending hold")
	}
	f.journal.release()
	return nil
}

func (f *credentialRunFixture) endingForwarded() bool {
	ending := f.journal.faultLog().Ending
	return ending != nil && !ending.Forwarded.IsZero()
}

func (f *credentialRunFixture) waitRecoveryUnlink(r *credentialRunProcess) (bool, error) {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		_, err := os.Lstat(r.staging)
		if os.IsNotExist(err) {
			f.recovery.absentAt = time.Now()
			return true, nil
		}
		if err != nil || f.endingForwarded() {
			return false, err
		}
		select {
		case <-r.done:
			return false, r.waitErr
		case <-tick.C:
		}
	}
}

func (w *world) credentialRunKilledAtBarrier(barrier string) error {
	f := w.credentialRun
	defer f.journal.clear()
	p, r, err := f.startRecovery(credentialRunFaults{Barrier: barrier})
	if err != nil {
		return err
	}
	worker, err := credentialRunAwaitWorker(r, f.journal.held())
	if err != nil || worker == "" {
		return err
	}
	f.recovery.killAt = time.Now()
	if err := r.process.Kill(); err != nil {
		return err
	}
	if err := f.awaitCredentialKill(r); err != nil {
		return err
	}
	f.journal.clear()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f.recovery.workerState, err = f.observeRecoveryWorker(ctx, p, r, worker)
	if err != nil {
		return err
	}
	f.recovery.stagingAtKill, err = f.newShmEntries()
	if err != nil || barrier != credentialStartBarrier || !f.recovery.workerState.Running {
		return err
	}
	id, err := f.prepareCredentialExec(ctx, p, worker, "/workspace/credrun-probe", "/workspace/"+credentialPlanName(3))
	if err != nil {
		return err
	}
	f.recovery.probe, err = f.readCredentialExec(ctx, id, p.plans[2])
	return err
}

func (f *credentialRunFixture) awaitCredentialKill(r *credentialRunProcess) error {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-r.done:
		if r.waitErr != nil {
			return r.waitErr
		}
		if !credentialRunWasKilled(r) {
			return errors.New("credential controller was reaped without the expected SIGKILL")
		}
	case <-timer.C:
		return errors.New("credential controller was not reaped within ten seconds")
	}
	select {
	case <-f.journal.faults.barrierEnded:
		if f.journal.faultLog().BarrierEnded.Before(f.recovery.killAt) {
			return errors.New("credential controller left the engine call before the harness kill")
		}
		return nil
	case <-timer.C:
		return errors.New("credential recorder did not observe the held request ending after the kill")
	}
}

func credentialRunWasKilled(r *credentialRunProcess) bool {
	if r.processState == nil {
		return false
	}
	status, ok := r.processState.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGKILL
}

func (w *world) credentialRunFreshRecovery() error {
	f := w.credentialRun
	if f.recovery.killAt.IsZero() {
		return nil
	}
	p := f.projects[0]
	if err := f.arm(); err != nil {
		return err
	}
	if err := p.observer.editPolicy("unlink", nil, nil); err != nil {
		var rejection credentialRunBracketRejection
		if !errors.As(err, &rejection) {
			return err
		}
		p.checkoutEditRejected = true
	}
	p.w.credentialApproval.observedInputs = []string{p.w.approval.record}
	finishWatch, err := p.w.credentialApprovalWatch()
	if err != nil {
		return err
	}
	f.recovery.policyAbsent = true
	r := p.runs[0]
	err = f.recoveryCommands(p, r, f.journal.faultLog().Worker, true)
	// Read the preservation snapshot after the product-only watch finishes.
	finishWatch()
	f.recovery.noInputAccess = p.w.credentialApproval.accessError == nil
	if err != nil {
		return err
	}
	return p.captureRecoveryApproval(r)
}

func (f *credentialRunFixture) recoveryCommands(p *credentialRunProject, r *credentialRunProcess, worker string, afterKill bool) error {
	rec := &f.recovery
	invoke := func(args ...string) *homeNativeResult {
		res := p.w.homeNativeInvoke(args...)
		if afterKill {
			_, err := os.Lstat(filepath.Join(p.w.doctor.checkout, "itos-container.yaml"))
			rec.policyAbsent = rec.policyAbsent && os.IsNotExist(err)
		}
		return &res
	}
	status := func() (credentialRunStatusPair, error) {
		pair := credentialRunStatusPair{plain: invoke("status"), json: invoke("status", "--json")}
		return pair, errors.Join(pair.plain.err, pair.json.err)
	}
	var err error
	rec.initial, err = status()
	if err != nil {
		return err
	}
	rec.final = rec.initial
	session := credentialRunStatusSession(rec.initial.json, r.session)
	if r.session != "" && (afterKill || session["stop_candidate"] == true) {
		rec.stop = invoke("stop", r.session)
		if rec.stop.err != nil {
			return rec.stop.err
		}
		rec.final, err = status()
		if err != nil {
			return err
		}
	}
	if afterKill {
		for _, exchange := range f.journal.snapshot() {
			if exchange.At.After(rec.killAt) {
				rec.afterKill = append(rec.afterKill, exchange)
			}
		}
	}
	if err := f.collectRecovery(p, r, worker); err != nil {
		return err
	}
	if !afterKill {
		return p.captureRecoveryApproval(r)
	}
	return nil
}

func (p *credentialRunProject) captureRecoveryApproval(r *credentialRunProcess) error {
	var err error
	r.approvalAfter, err = os.ReadFile(p.w.approval.record)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (f *credentialRunFixture) collectRecovery(p *credentialRunProject, r *credentialRunProcess, worker string) error {
	rec := &f.recovery
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	rec.owned, err = p.engine(f.cli).owned(ctx)
	if err != nil {
		return err
	}
	rec.shm, err = f.newShmEntries()
	if err != nil {
		return err
	}
	if credentialStageFile(r.staging) {
		_, err := os.Lstat(filepath.Dir(r.staging))
		rec.directoryAbsent = os.IsNotExist(err)
		if err != nil && !rec.directoryAbsent {
			return err
		}
	}
	_, err = f.cli.ContainerInspect(ctx, worker, client.ContainerInspectOptions{})
	rec.workerAbsent = errdefs.IsNotFound(err)
	if err != nil && !rec.workerAbsent {
		return err
	}
	rec.collected = true
	r.clean = rec.allRemoved()
	return nil
}

func (r *credentialRunRecovery) allRemoved() bool {
	return r.collected && r.workerAbsent && r.directoryAbsent && len(r.owned) == 0 && len(r.shm) == 0
}

func credentialRunZeroStart(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	start, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && start.IsZero()
}
