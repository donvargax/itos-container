package features

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type credentialRunExec struct {
	started, finished time.Time
	first, final      credentialRunReport
	out, diagnostic   []byte
	exit              int
}

func (w *world) credentialRunDetachedPlans() error {
	f := w.credentialRun
	p := f.projects[0]
	detached := credentialRunPlan{Nonce: egressNonce(), Target: credentialRunTarget, Detached: true}
	read := credentialRunPlan{Nonce: egressNonce(), Target: credentialRunTarget, Forbidden: []string{p.source, p.sibling}}
	if err := p.writePlan(detached); err != nil {
		return err
	}
	if err := p.writePlan(read); err != nil {
		return err
	}
	if err := f.detachedControl(p, detached, read); err != nil {
		return err
	}
	return f.positiveDetectors()
}

func (f *credentialRunFixture) detachedControl(p *credentialRunProject, detached, read credentialRunPlan) (result error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := filepath.Join(f.root, "detached-control")
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	target := filepath.Join(root, "control")
	body, err := f.writeMarker(target, 0400)
	if err != nil {
		return err
	}
	read.Forbidden = nil
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: target, Target: credentialRunTarget, ReadOnly: true},
		{Type: mount.TypeBind, Source: filepath.Join(p.w.doctor.checkout, "credrun-probe"), Target: "/credrun-probe", ReadOnly: true},
	}
	for name, plan := range map[string]credentialRunPlan{"credrun-plan.json": detached, "credrun-exec-plan.json": read} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, approvalJSON(plan), 0600); err != nil {
			return err
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: path, Target: "/" + name, ReadOnly: true})
	}
	created, err := f.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: "credrun-detached-control-" + detached.Nonce,
		Config: &container.Config{
			Image: p.w.doctor.imageID, User: p.w.approval.uid + ":" + p.w.approval.gid,
			Entrypoint: []string{}, Cmd: []string{"/credrun-probe", "/credrun-plan.json"},
			Labels: map[string]string{"credrun.control": detached.Nonce}, Healthcheck: &container.HealthConfig{Test: []string{"NONE"}},
		},
		HostConfig: &container.HostConfig{
			NetworkMode: "none", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"},
			Mounts: mounts,
		},
	})
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := f.cli.ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true})
		result = errors.Join(result, err)
	}()
	if _, err := f.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return err
	}
	if err := f.detachedControlRunning(ctx, created.ID, 200*time.Millisecond); err != nil {
		return err
	}
	if _, err := f.cli.ContainerKill(ctx, created.ID, client.ContainerKillOptions{Signal: "SIGTERM"}); err != nil {
		return err
	}
	if err := f.detachedControlRunning(ctx, created.ID, 300*time.Millisecond); err != nil {
		return err
	}
	execID, err := f.prepareCredentialExec(ctx, p, created.ID, "/credrun-probe", "/credrun-exec-plan.json")
	if err != nil {
		return err
	}
	probe, err := f.readCredentialExec(ctx, execID, read)
	if err != nil {
		return err
	}
	hash := credentialRunHash(body)
	if probe.exit != 0 || !probe.first.hasFile(hash) || !probe.final.hasFile(hash) || len(probe.diagnostic) != 0 {
		return errors.New("detached control exec did not report the expected hash and exit")
	}
	if err := f.signalCredentialExit(ctx, created.ID); err != nil {
		return err
	}
	stream, err := f.cli.ContainerLogs(ctx, created.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	if n, err := io.Copy(io.Discard, io.LimitReader(stream, 1)); err != nil || n != 0 {
		return errors.New("detached probe control did not have an empty log")
	}
	return nil
}

func (f *credentialRunFixture) detachedControlRunning(ctx context.Context, worker string, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	state, err := f.credentialWorkerState(ctx, worker)
	if err != nil {
		return err
	}
	if !state.Running {
		return errors.New("detached control worker stopped before SIGUSR1")
	}
	return nil
}

func (f *credentialRunFixture) prepareCredentialExec(ctx context.Context, p *credentialRunProject, worker, probe, plan string) (string, error) {
	created, err := f.cli.ExecCreate(ctx, worker, client.ExecCreateOptions{
		User: p.w.approval.uid + ":" + p.w.approval.gid, Cmd: []string{probe, plan},
		AttachStdin: true, AttachStdout: true, AttachStderr: true,
	})
	return created.ID, err
}

func (f *credentialRunFixture) readCredentialExec(ctx context.Context, id string, plan credentialRunPlan) (credentialRunExec, error) {
	probe := credentialRunExec{started: time.Now()}
	stream, err := f.cli.ExecAttach(ctx, id, client.ExecAttachOptions{})
	if err != nil {
		return probe, err
	}
	defer stream.Close()
	unwatch := context.AfterFunc(ctx, stream.Close)
	defer unwatch()
	if _, err := io.WriteString(stream.Conn, "exit 0\n"); err != nil {
		return probe, err
	}
	const limit = 1 << 20
	reader := &io.LimitedReader{R: stream.Reader, N: limit + 1}
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, reader); err != nil || reader.N == 0 {
		return probe, errors.New("credential exec capture was incomplete")
	}
	probe.out, probe.diagnostic = stdout.Bytes(), stderr.Bytes()
	probe.first, _ = credentialReport(probe.out, plan.Nonce, "first")
	probe.final, _ = credentialReport(probe.out, plan.Nonce, "final")
	// Docker closes the attach stream as it publishes the exec exit. Wait for
	// the inspect result to settle within the same deadline as the exec.
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		result, err := f.cli.ExecInspect(ctx, id, client.ExecInspectOptions{})
		if err != nil {
			return probe, err
		}
		if !result.Running {
			probe.exit, probe.finished = result.ExitCode, time.Now()
			return probe, nil
		}
		select {
		case <-ctx.Done():
			return probe, ctx.Err()
		case <-tick.C:
		}
	}
}

func (f *credentialRunFixture) credentialWorkerState(ctx context.Context, worker string) (*container.State, error) {
	result, err := f.cli.ContainerInspect(ctx, worker, client.ContainerInspectOptions{})
	if err != nil {
		return nil, err
	}
	if result.Container.State == nil {
		return nil, errors.New("credential worker inspection lacked state")
	}
	return result.Container.State, nil
}

func (f *credentialRunFixture) signalCredentialExit(parent context.Context, worker string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if _, err := f.cli.ContainerKill(ctx, worker, client.ContainerKillOptions{Signal: "SIGUSR1"}); err != nil {
		return err
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		state, err := f.credentialWorkerState(ctx, worker)
		if err != nil {
			return err
		}
		if !state.Running {
			if state.ExitCode != 23 {
				return errors.New("credential worker did not exit 23 after SIGUSR1")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
