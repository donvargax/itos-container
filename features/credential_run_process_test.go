package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

type credentialRunAccess struct {
	Kind, Hash, Mode, Errno string
	UID                     uint32
}

type credentialRunReport struct {
	Nonce, Phase                  string
	UID, GID                      int
	Target                        credentialRunAccess
	Forbidden                     map[string]credentialRunAccess
	Writes                        map[string]string
	CredentialEntries, ShmEntries []string
	CredentialListing, ShmListing string
	TokenEnv, MarkerInArgsOrEnv   bool
	Child                         egressProbeReport
	ChildError                    string
}

func (r credentialRunReport) hasFile(hash string) bool {
	return r.Nonce != "" && r.UID == os.Geteuid() && r.GID == os.Getegid() &&
		r.Target.Kind == "file" && r.Target.Hash == hash && r.Target.Mode == "0400" && r.Target.UID == uint32(os.Geteuid())
}

func (r credentialRunReport) writesDenied() bool {
	for _, op := range []string{"open", "create", "unlink", "rename"} {
		switch r.Writes[op] {
		case "EACCES", "EPERM", "EROFS", "EBUSY":
		default:
			return false
		}
	}
	return true
}

func credentialReport(out []byte, nonce, phase string) (credentialRunReport, bool) {
	var matched credentialRunReport
	for _, line := range bytes.Split(out, []byte("\n")) {
		var report credentialRunReport
		if json.Unmarshal(line, &report) != nil || report.Nonce != nonce || report.Phase != phase {
			continue
		}
		if matched.Nonce != "" || report.Target.Kind == "file" && len(report.Target.Hash) != 64 {
			return credentialRunReport{}, false
		}
		matched = report
	}
	return matched, matched.Nonce != ""
}

type credentialRunProcess struct {
	ctx                                     context.Context
	cancel                                  context.CancelFunc
	input                                   io.WriteCloser
	done                                    chan struct{}
	out, diagnostic                         offlineBuffer
	waitErr                                 error
	exit                                    int
	plan                                    credentialRunPlan
	first, read, final                      credentialRunReport
	observed                                *egressObserved
	workerRaw, proxyRaw                     json.RawMessage
	session                                 string
	staging, retainedLog                    string
	stagingAbsentLive, clean, finished      bool
	status, jsonStatus                      *homeNativeResult
	proxyOut, proxyErr, retained            []byte
	approvalBefore, approvalAfter           []byte
	noSourceAccess, peerSurvived, noStaging bool
}

func (p *credentialRunProject) start(index int) (*credentialRunProcess, error) {
	if index < 1 || index > len(p.plans) {
		return nil, errors.New("credential run plan was not prepared in Given")
	}
	plan := p.plans[index-1]
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	run := &credentialRunProcess{ctx: ctx, cancel: cancel, done: make(chan struct{}), exit: -1, plan: plan}
	approval, err := os.ReadFile(p.w.approval.record)
	if err != nil {
		cancel()
		return nil, err
	}
	run.approvalBefore = approval
	cmd := exec.CommandContext(ctx, p.w.bin, "run", "--", "/workspace/credrun-probe", "/workspace/"+credentialPlanName(index))
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = p.w.doctor.checkout, p.w.env(), &run.out, &run.diagnostic
	cmd.WaitDelay = time.Second
	run.input, err = cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		cancel()
		return nil, err
	}
	p.runs = append(p.runs, run)
	go func() {
		err := cmd.Wait()
		run.exit = cmd.ProcessState.ExitCode()
		var status *exec.ExitError
		if err != nil && !errors.As(err, &status) {
			run.waitErr = err
		}
		run.waitErr = errors.Join(run.waitErr, ctx.Err())
		close(run.done)
	}()
	return run, nil
}

func (r *credentialRunProcess) await(phase string) (credentialRunReport, error) {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if report, ok := credentialReport(r.out.bytes(), r.plan.Nonce, phase); ok {
			return report, nil
		}
		select {
		case <-r.done:
			report, _ := credentialReport(r.out.bytes(), r.plan.Nonce, phase)
			return report, r.waitErr
		case <-tick.C:
		}
	}
}

func (r *credentialRunProcess) send(command string) error {
	select {
	case <-r.done:
		return r.waitErr
	default:
	}
	if _, err := io.WriteString(r.input, command+"\n"); err != nil {
		// A product exit during a barrier command is still a product outcome.
		select {
		case <-r.done:
			return r.waitErr
		case <-r.ctx.Done():
			return r.ctx.Err()
		}
	}
	return nil
}

func (f *credentialRunFixture) observe(p *credentialRunProject, r *credentialRunProcess) error {
	driver := &egressNativeRun{n: p.engine(f.cli), observed: map[string]*egressObserved{}}
	driver.observe("credential run barrier")
	r.observed = driver.observed["credential run barrier"]
	if err := r.observed.err; err != nil {
		return err
	}
	if r.observed.worker != nil {
		id := r.observed.worker.ID
		if err := p.w.doctor.real.read("/containers/"+id+"/json", &r.workerRaw, 200); err != nil {
			return err
		}
		// Missing registration is a product observation. The associated assertions
		// require a nonempty session, and do not invent one from inspect output.
		session, _, err := p.w.homeNativeRegistration(credentialRunObject(r.workerRaw))
		if err == nil {
			r.session = session
			root := filepath.Join(f.state, "itos-container", "sessions", p.w.recoveryIndexKey())
			r.retainedLog = filepath.Join(root, session, "egress-refusals.jsonl")
		}
		for _, m := range r.observed.worker.Mounts {
			if m.Destination == credentialRunTarget {
				r.staging = m.Source
			}
		}
	}
	if r.observed.proxy != nil {
		id := r.observed.proxy.ID
		if err := p.w.doctor.real.read("/containers/"+id+"/json", &r.proxyRaw, 200); err != nil {
			return err
		}
		if err := f.readProxyLog(r, id); err != nil {
			return err
		}
	}
	return nil
}

func credentialRunObject(data []byte) map[string]any {
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}

func (f *credentialRunFixture) readProxyLog(r *credentialRunProcess, id string) error {
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()
	stream, err := f.cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	var stdout, stderr bytes.Buffer
	n, err := stdcopy.StdCopy(&stdout, &stderr, io.LimitReader(stream, credentialRunBodyLimit+1))
	if err != nil || n >= credentialRunBodyLimit {
		return errors.New("proxy container log capture was incomplete")
	}
	r.proxyOut, r.proxyErr = stdout.Bytes(), stderr.Bytes()
	return nil
}

func (f *credentialRunFixture) connectEgress(p *credentialRunProject, r *credentialRunProcess) error {
	if f.egress == nil {
		return nil
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	filter := make(client.Filters).Add("label", "io.itos-container.project="+p.w.approval.project).Add("label", "io.itos-container.kind=egress-proxy")
	for {
		select {
		case <-r.done:
			return r.waitErr
		case <-r.ctx.Done():
			return r.ctx.Err()
		case <-tick.C:
		}
		found, err := f.cli.ContainerList(r.ctx, client.ContainerListOptions{Filters: filter})
		if err != nil {
			return err
		}
		for _, item := range found.Items {
			if item.Labels["io.itos-container.project"] != p.w.approval.project || item.Labels["io.itos-container.kind"] != "egress-proxy" {
				continue
			}
			for _, network := range f.egress.networks {
				if _, err := f.cli.NetworkConnect(r.ctx, network.id, client.NetworkConnectOptions{Container: item.ID}); err != nil {
					return err
				}
			}
			return r.send("network-ready")
		}
	}
}

func (f *credentialRunFixture) waitForStartupUnlink(r *credentialRunProcess) (bool, error) {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		entries, err := f.newShmEntries()
		if err != nil || len(entries) == 0 {
			return err == nil, err
		}
		select {
		case <-r.done:
			return false, r.waitErr
		case <-deadline.C:
			return false, nil
		case <-tick.C:
		}
	}
}

func (f *credentialRunFixture) finish(p *credentialRunProject, r *credentialRunProcess, exit *int) error {
	if r.finished {
		return nil
	}
	if exit != nil && r.first.Nonce != "" {
		if err := r.send(fmt.Sprintf("exit %d", *exit)); err != nil {
			return err
		}
	}
	<-r.done
	if r.waitErr != nil {
		return r.waitErr
	}
	r.cancel()
	r.final, _ = credentialReport(r.out.bytes(), r.plan.Nonce, "final")
	if r.retainedLog != "" {
		var err error
		r.retained, err = os.ReadFile(r.retainedLog)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	owned, err := p.engine(f.cli).owned(ctx)
	if err != nil {
		return err
	}
	shm, err := f.newShmEntries()
	if err != nil {
		return err
	}
	for _, peer := range f.projects {
		if peer == p {
			continue
		}
		for _, live := range peer.runs {
			select {
			case <-live.done:
			default:
				if credentialStageFile(live.staging) {
					delete(shm, filepath.Base(filepath.Dir(live.staging)))
				}
			}
		}
	}
	r.clean = len(owned) == 0 && len(shm) == 0
	r.approvalAfter, err = os.ReadFile(p.w.approval.record)
	r.finished = err == nil
	return err
}

func credentialRunFailure(r *credentialRunProcess, detail string) error {
	select {
	case <-r.done:
		rule := "unreported"
		diagnostic := r.diagnostic.bytes()
		if bytes.Contains(diagnostic, []byte("policy.version")) && bytes.Contains(diagnostic, []byte("/version")) {
			rule = "policy.version at /version"
		}
		return fmt.Errorf("%s: run exited %d (%s)", detail, r.exit, rule)
	default:
		return fmt.Errorf("%s: live run lacked the expected observation", detail)
	}
}

func (p *credentialRunProject) runDoctor() error {
	res := p.w.homeNativeInvoke("doctor", "--json")
	p.doctor = &res
	return res.err
}

func (p *credentialRunProject) inspectBeforeRun() error {
	var err error
	path := filepath.Dir(filepath.Dir(filepath.Dir(p.w.approval.record)))
	p.approvalBeforeTools, err = doctorTree(path)
	if err != nil {
		return err
	}
	if err := p.runDoctor(); err != nil {
		return err
	}
	plain := p.w.homeNativeInvoke("doctor")
	p.plainDoctor = &plain
	if plain.err != nil {
		return plain.err
	}
	if err := p.w.credentialApprovalTerminal("plain", "unavailable", "", ""); err != nil {
		return err
	}
	p.approve = &homeNativeResult{exit: p.w.exit, out: []byte(p.w.stdout), errout: []byte(p.w.stderr)}
	if strings.Contains(p.w.approval.beforeReview, "y/N") {
		return errors.New("matching terminal approval waited for consent")
	}
	p.approvalAfterTools, err = doctorTree(path)
	return err
}
