package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/mount"
)

func (w *world) credentialRunReady() error {
	f := w.credentialRun
	if !f.verified || !f.controlled {
		return errors.New("native fixture verification or positive controls were not recorded")
	}
	for i, p := range f.projects {
		if p.doctor == nil || p.doctor.exit != 0 {
			return fmt.Errorf("project %d JSON doctor did not exit successfully", i+1)
		}
		result := credentialRunObject(p.doctor.out)
		inspection, approval := object(result["inspection"]), object(result["approval"])
		problems, list := result["problems"].([]any)
		if result["ok"] != true || !list || len(problems) != 0 || inspection["ready"] != true ||
			len(object(inspection["checkout"])) == 0 || len(object(inspection["engine"])) == 0 ||
			len(object(inspection["image"])) == 0 || approval["status"] != "matching" {
			return fmt.Errorf("project %d did not report full readiness and matching approval", i+1)
		}
		if _, issues := result["credentials"]; issues {
			return fmt.Errorf("project %d reported unexpected credential findings", i+1)
		}
		effective := object(approval["effective"])
		if effective["profile"] != p.w.approval.settings["profile"] ||
			!bytes.Equal(approvalJSON(effective["credentials"]), approvalJSON(p.w.approval.settings["credentials"])) {
			return fmt.Errorf("project %d did not report exactly the independently approved credential profile and grants", i+1)
		}
		_, present := effective["credentials"]
		if present != (p.source != "") {
			return fmt.Errorf("project %d reported unexpected credential grant presence", i+1)
		}
	}
	return nil
}

func (w *world) credentialRunFirstHash() error {
	p := w.credentialRun.projects[0]
	r := p.runs[0]
	if !r.first.hasFile(credentialRunHash(p.originalSource)) {
		return credentialRunFailure(r, "worker did not report the approved source hash")
	}
	return nil
}

func credentialStageFile(path string) bool {
	parent := filepath.Dir(path)
	return filepath.Dir(parent) == "/dev/shm" && credentialStageName(filepath.Base(parent)) && filepath.Base(path) == "claude-token"
}

func credentialStageName(name string) bool {
	suffix, ok := strings.CutPrefix(name, "itos-container-")
	return ok && len(suffix) == 32 && strings.Trim(suffix, "0123456789abcdef") == ""
}

func credentialRunMounts(r *credentialRunProcess, grant bool) bool {
	if r.observed == nil || r.observed.worker == nil || r.observed.worker.HostConfig == nil {
		return false
	}
	c := r.observed.worker
	checkout, credential := 0, 0
	for _, m := range c.Mounts {
		switch {
		case m.Destination == "/workspace" && m.Type == mount.TypeVolume && m.RW:
			checkout++
		case m.Destination == "/tmp" && m.Type == mount.TypeTmpfs && m.RW:
		case m.Destination == credentialRunTarget && grant && m.Type == mount.TypeBind && !m.RW && credentialStageFile(m.Source):
			credential++
		default:
			return false
		}
	}
	want := 0
	if grant {
		want = 1
	}
	if checkout != 1 || credential != want || len(c.HostConfig.Mounts) != 1+want {
		return false
	}
	checkout, credential = 0, 0
	for _, m := range c.HostConfig.Mounts {
		switch {
		case m.Target == "/workspace" && m.Type == mount.TypeVolume && !m.ReadOnly && m.VolumeOptions != nil && m.VolumeOptions.NoCopy:
			checkout++
		case m.Target == credentialRunTarget && grant && m.Type == mount.TypeBind && m.ReadOnly && m.Source == r.staging:
			credential++
		default:
			return false
		}
	}
	return checkout == 1 && credential == want
}

func (w *world) credentialRunHardening() error {
	p := w.credentialRun.projects[0]
	r := p.runs[0]
	if !credentialRunMounts(r, true) || r.first.CredentialListing != "" || !slices.Equal(r.first.CredentialEntries, []string{"claude-token"}) ||
		r.first.ShmListing != "" || len(r.first.ShmEntries) != 0 || !r.first.hasFile(credentialRunHash(p.originalSource)) {
		return errors.New("inspect and worker probe did not confirm exactly one private read-only mode-0400 credential mount")
	}
	c := r.observed.worker
	h, config := c.HostConfig, c.Config
	if config == nil || config.User != p.w.approval.uid+":"+p.w.approval.gid || config.WorkingDir != "/workspace" ||
		config.Tty || !config.OpenStdin || !config.StdinOnce || !config.AttachStdin || !config.AttachStdout || !config.AttachStderr ||
		len(config.Entrypoint) != 0 || !slices.Equal(config.Cmd, []string{"/workspace/credrun-probe", "/workspace/credrun-plan-1.json"}) ||
		config.Healthcheck == nil || !slices.Equal(config.Healthcheck.Test, []string{"NONE"}) || c.RestartCount != 0 {
		return errors.New("worker inspect differed from the literal argv and host identity contract")
	}
	if !h.ReadonlyRootfs || h.Privileged || h.AutoRemove || h.PublishAllPorts || h.IpcMode != "private" || h.CgroupnsMode != "private" ||
		h.PidMode != "" || h.UTSMode != "" || h.UsernsMode != "" || h.Init == nil || *h.Init || h.RestartPolicy.Name != "no" ||
		h.PidsLimit == nil || *h.PidsLimit != 4096 || !slices.Equal(h.CapDrop, []string{"ALL"}) || len(h.CapAdd) != 0 || len(h.GroupAdd) != 0 ||
		len(h.Devices) != 0 || len(h.DeviceRequests) != 0 || len(h.DeviceCgroupRules) != 0 || len(h.PortBindings) != 0 ||
		len(h.Binds) != 0 || len(h.VolumesFrom) != 0 || len(h.Links) != 0 || len(h.ExtraHosts) != 0 || len(h.Sysctls) != 0 ||
		len(h.SecurityOpt) != 1 || (h.SecurityOpt[0] != "no-new-privileges" && h.SecurityOpt[0] != "no-new-privileges=true") ||
		h.LogConfig.Type != "none" || len(h.Tmpfs) != 1 || h.Tmpfs["/tmp"] != "rw,exec,nosuid,nodev,mode=1777" {
		return errors.New("worker inspection differed from the hardened runtime contract")
	}
	return nil
}

func credentialCannotOpen(report credentialRunReport, paths ...string) bool {
	for _, path := range paths {
		a := report.Forbidden[path]
		if a.Kind != "absent" && a.Kind != "denied" {
			return false
		}
	}
	return true
}

func (w *world) credentialRunIsolation() error {
	p := w.credentialRun.projects[0]
	r := p.runs[0]
	if !r.first.writesDenied() || !credentialCannotOpen(r.first, p.source, p.sibling) ||
		r.first.ShmListing != "" || slices.Contains(r.first.ShmEntries, filepath.Base(filepath.Dir(r.staging))) {
		return errors.New("worker probe did not confirm write refusal and isolation from source, sibling and host staging")
	}
	return nil
}

func (w *world) credentialRunEnvironment() error {
	f := w.credentialRun
	r := f.projects[0].runs[0]
	if r.first.Nonce == "" || r.first.TokenEnv || r.first.MarkerInArgsOrEnv || r.observed == nil || r.observed.worker == nil || r.observed.worker.Config == nil {
		return errors.New("credential environment and argv were absent or not isolated")
	}
	c := r.observed.worker
	for _, value := range append(slices.Clone(c.Config.Env), c.Config.Cmd...) {
		if strings.HasPrefix(value, "CLAUDE_CODE_OAUTH_TOKEN=") {
			return errors.New("controller automatically exported the credential")
		}
		if err := f.scan("worker environment or argv", []byte(value)); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) credentialRunNetwork(expected string) error {
	f := w.credentialRun
	r := f.projects[0].runs[0]
	if r.observed == nil || r.observed.worker == nil || r.observed.worker.HostConfig == nil {
		return errors.New("worker network was not observed")
	}
	if expected == "network none with no proxy or refusal log" {
		if f.egress != nil || r.observed.worker.HostConfig.NetworkMode != "none" || r.observed.proxy != nil || len(r.observed.networks) != 0 || len(r.retained) != 0 ||
			f.hasRetainedLog() {
			return errors.New("offline worker used unexpected egress resources")
		}
		return nil
	}
	if expected != "successful controlled TLS and an ordinary denied hostname in all log copies" || f.egress == nil {
		return errors.New("fixture does not match required allowlist network evidence")
	}
	results := r.first.Child.Results
	if r.observed.proxy == nil || r.first.ChildError != "" || !r.first.Child.Done || len(results) != 2 {
		return errors.New("allowlist probe or proxy observation was incomplete")
	}
	allowed, refused := results[0], results[1]
	if !allowed.OK || allowed.Status != 200 || allowed.Peer != f.endpoint.leaf || allowed.Reply != f.endpoint.nonce ||
		!refused.OK || refused.Target != "refused.example:443" || refused.Status != 403 || !refused.Closed || refused.Body != 0 {
		return errors.New("worker did not prove controlled TLS and ordinary proxy refusal")
	}
	if !credentialOrdinaryRefusal(r.proxyOut) || !credentialOrdinaryRefusal(r.retained) || !f.proxyStdoutJournal(r.observed.proxy.ID) {
		return errors.New("ordinary refusal is missing from one of the observed proxy log copies")
	}
	return nil
}

func credentialOrdinaryRefusal(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		value := credentialRunObject(line)
		if value["event"] == "refused" && value["method"] == "CONNECT" && value["target"] == "refused.example" &&
			value["port"] == "443" && value["reason"] == "not-listed" {
			return true
		}
	}
	return false
}

func (f *credentialRunFixture) proxyStdoutJournal(id string) bool {
	for _, exchange := range f.journal.snapshot() {
		if !strings.HasSuffix(exchange.Path, "/containers/"+id+"/logs") || exchange.Status != 200 ||
			!strings.Contains(exchange.Query, "stdout=1") || strings.Contains(exchange.Query, "stderr=1") {
			continue
		}
		var stdout bytes.Buffer
		if _, err := stdcopy.StdCopy(&stdout, io.Discard, bytes.NewReader(exchange.ResponseBody)); err == nil &&
			credentialOrdinaryRefusal(stdout.Bytes()) {
			return true
		}
	}
	return false
}

func (f *credentialRunFixture) hasRetainedLog() bool {
	for _, root := range []string{f.state, f.fixedState()} {
		present := false
		err := filepath.WalkDir(root, func(_ string, e fs.DirEntry, err error) error {
			if err == nil && e.Name() == "egress-refusals.jsonl" {
				present = true
			}
			return err
		})
		if err != nil || present {
			return true
		}
	}
	return false
}

func (w *world) credentialRunUnlinked() error {
	p := w.credentialRun.projects[0]
	r := p.runs[0]
	if !r.stagingAbsentLive || !credentialStageFile(r.staging) || !r.read.hasFile(credentialRunHash(p.originalSource)) {
		return errors.New("credential staging absence and live readable copy were not both observed")
	}
	return nil
}

func (w *world) credentialRunExit(exit int) error {
	r := w.credentialRun.projects[0].runs[0]
	if !r.clean || !r.finished || r.session == "" || r.final.Nonce == "" || r.exit != exit {
		return credentialRunFailure(r, "run did not return the observed exit after owned cleanup")
	}
	return nil
}

func credentialOriginalFile(path string, original []byte, before os.FileInfo) bool {
	actual, err := os.Lstat(path)
	if err != nil || before == nil || !actual.Mode().IsRegular() || actual.Mode() != before.Mode() || !os.SameFile(before, actual) {
		return false
	}
	content, err := os.ReadFile(path)
	return err == nil && bytes.Equal(content, original)
}

func (w *world) credentialRunUnchanged() error {
	f := w.credentialRun
	p := f.projects[0]
	got, err := doctorTree(p.w.doctor.checkout)
	if err != nil {
		return err
	}
	engine, err := p.w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(p.checkoutBefore, got) || !reflect.DeepEqual(f.originalResources, engine) ||
		!credentialOriginalFile(p.source, p.originalSource, p.sourceInfo) || !credentialOriginalFile(p.sibling, p.originalSibling, p.siblingInfo) {
		return errors.New("checkout, original sources or unrelated engine resources changed")
	}
	return w.credentialRunApprovals()
}

func (w *world) credentialRunOriginalCopy() error {
	f := w.credentialRun
	p := f.projects[0]
	r := p.runs[0]
	hash, rotated := credentialRunHash(p.originalSource), credentialRunHash(f.rotated)
	if hash == rotated || !r.first.hasFile(hash) || !r.read.hasFile(hash) || f.sourceAfterRotation != rotated {
		return credentialRunFailure(r, "worker did not retain the original hash through source rotation")
	}
	return nil
}

func (w *world) credentialRunRotatedCopy() error {
	f := w.credentialRun
	p := f.projects[0]
	before, after := p.runs[0], p.runs[1]
	if !after.first.hasFile(credentialRunHash(f.rotated)) || !credentialStageFile(before.staging) || !credentialStageFile(after.staging) ||
		before.staging == after.staging || before.session == "" || after.session == "" || before.session == after.session {
		return credentialRunFailure(after, "fresh worker did not report the rotated hash from its own staging")
	}
	return nil
}

func (w *world) credentialRunApprovals() error {
	for _, p := range w.credentialRun.projects {
		for _, r := range p.runs {
			before := credentialRunObject(r.approvalBefore)
			if !r.finished || before["fingerprint"] != p.w.approval.fingerprint || before["generation"] != p.w.approval.generation ||
				!bytes.Equal(r.approvalBefore, r.approvalAfter) || bytes.Contains(r.diagnostic.bytes(), []byte("y/N")) ||
				bytes.Contains(r.out.bytes(), []byte("y/N")) {
				return errors.New("approval generation or fingerprint changed, or run prompted for consent")
			}
		}
	}
	return nil
}

func (w *world) credentialRunSeparateGrants() error {
	f := w.credentialRun
	for i, p := range f.projects[:2] {
		r := p.runs[0]
		if !r.first.hasFile(credentialRunHash(p.originalSource)) || !credentialCannotOpen(r.first, f.projects[1-i].source) {
			return credentialRunFailure(r, "credential worker did not report only its own source hash")
		}
	}
	for i, p := range f.projects[:2] {
		r, other := p.runs[0], f.projects[1-i].runs[0]
		if !credentialRunMounts(r, true) || r.staging == other.staging || r.first.ShmListing != "" ||
			slices.Contains(r.first.ShmEntries, filepath.Base(filepath.Dir(other.staging))) {
			return errors.New("credential workers did not prove separate staging and private shared memory")
		}
	}
	return nil
}

func credentialRunNoGrant(r *credentialRunProcess) bool {
	return r.first.Nonce != "" && r.first.UID == os.Geteuid() && !r.first.TokenEnv && !r.first.MarkerInArgsOrEnv &&
		(r.first.Target.Kind == "absent" || r.first.Target.Kind == "denied") && credentialCannotOpen(r.first, r.plan.Forbidden...) &&
		credentialRunMounts(r, false)
}

func (w *world) credentialRunThirdIsolated() error {
	r := w.credentialRun.projects[2].runs[0]
	if !credentialRunNoGrant(r) {
		return credentialRunFailure(r, "third worker did not prove source and fixed-path isolation")
	}
	return nil
}

func (w *world) credentialRunStoppedFirst() error {
	f := w.credentialRun
	first, second, third := f.projects[0], f.projects[1], f.projects[2]
	if first.stop == nil || first.stop.exit != 0 || !first.runs[0].clean || !second.runs[0].peerSurvived || !third.runs[0].peerSurvived ||
		!second.runs[0].read.hasFile(credentialRunHash(second.originalSource)) {
		return errors.New("stop did not remove just the first project's resources while its peers remained active")
	}
	return nil
}

func (w *world) credentialRunLaterSession() error {
	f := w.credentialRun
	r := f.later
	if !credentialRunNoGrant(r) || r.session == "" || r.session == f.projects[0].runs[0].session {
		return credentialRunFailure(r, "later run did not prove a fresh no-grant session")
	}
	return nil
}

func (w *world) credentialRunLaterIsolated() error {
	r := w.credentialRun.later
	if !credentialRunNoGrant(r) || !r.noSourceAccess || !r.noStaging || r.staging != "" || r.exit != 0 || r.final.Nonce == "" {
		return credentialRunFailure(r, "later probe did not prove isolation from both sources and staging")
	}
	return nil
}

func (w *world) credentialRunAllClean() error {
	f := w.credentialRun
	for _, p := range f.projects {
		for index, r := range p.runs {
			if !r.clean || !r.finished || r.session == "" || (index != 0 || p.stop == nil) && (r.final.Nonce == "" || r.exit != 0) {
				return credentialRunFailure(r, "owned cleanup or workload completion was not observed")
			}
		}
		if p.source == "" {
			continue
		}
		if f.rotation == "" {
			if !credentialOriginalFile(p.source, p.originalSource, p.sourceInfo) {
				return errors.New("one project's original source changed")
			}
		} else if !credentialOriginalFile(p.source, f.rotated, f.rotatedInfo) {
			return errors.New("the rotated source changed during the later run")
		}
	}
	return w.credentialRunSurfaces()
}

func (f *credentialRunFixture) scan(surface string, data []byte) error {
	if hits := f.hits(data); len(hits) != 0 {
		return fmt.Errorf("synthetic credential detector %s matched %s", hits[0], surface)
	}
	return nil
}

func (f *credentialRunFixture) scanTranscript(surface string, result *homeNativeResult) error {
	if result == nil || result.err != nil || len(result.out)+len(result.errout) == 0 {
		return fmt.Errorf("missing %s transcript", surface)
	}
	return errors.Join(f.scan(surface+" stdout", result.out), f.scan(surface+" stderr", result.errout))
}

func (w *world) credentialRunSurfaces() error {
	f := w.credentialRun
	if !f.controlled || len(f.journal.snapshot()) == 0 {
		return errors.New("marker controls or engine journal are missing")
	}
	if err := f.journal.scan(f.scan); err != nil {
		return err
	}
	roots := []string{f.state, f.fixedState()}
	for i, p := range f.projects {
		if p.checkoutEditRejected {
			return errors.New("checkout policy edit did not preserve its bracket or staged identity")
		}
		if err := p.observer.judge(); err != nil {
			if leak := f.scan("checkout observer diagnostic", []byte(err.Error())); leak != nil {
				return leak
			}
			return err
		}
		wantEdits := 0
		if i == 0 && f.later != nil {
			wantEdits = 1
		}
		if len(p.observer.mutations) != wantEdits {
			return errors.New("checkout policy mutation count differed from the scenario")
		}
		if err := f.scanTranscript("JSON doctor", p.doctor); err != nil {
			return err
		}
		if p.plainDoctor != nil || p.approve != nil {
			if err := errors.Join(f.scanTranscript("plain doctor", p.plainDoctor), f.scanTranscript("terminal approval", p.approve)); err != nil {
				return err
			}
			if p.plainDoctor.exit != 0 || p.approve.exit != 0 || !reflect.DeepEqual(p.approvalBeforeTools, p.approvalAfterTools) {
				return errors.New("doctor or matching terminal approval failed or published state")
			}
			r := p.runs[0]
			if err := errors.Join(f.scanTranscript("plain active status", r.status), f.scanTranscript("JSON active status", r.jsonStatus)); err != nil {
				return err
			}
			if r.status.exit != 0 || r.jsonStatus.exit != 0 || !json.Valid(r.jsonStatus.out) ||
				r.session == "" || !bytes.Contains(r.status.out, []byte(r.session)) || !bytes.Contains(r.jsonStatus.out, []byte(r.session)) {
				return errors.New("status did not report the active session successfully")
			}
		}
		if p.stop != nil {
			if err := f.scanTranscript("stop", p.stop); err != nil {
				return err
			}
		}
		for _, r := range p.runs {
			if !r.finished || r.first.Nonce == "" || !json.Valid(r.workerRaw) {
				return errors.New("run transcript or raw worker inspection is missing")
			}
			for name, data := range map[string][]byte{
				"run stdout": r.out.bytes(), "run stderr": r.diagnostic.bytes(), "worker inspect": r.workerRaw,
			} {
				if err := f.scan(name, data); err != nil {
					return err
				}
			}
			if f.egress != nil {
				if !json.Valid(r.proxyRaw) || len(r.proxyOut) == 0 || len(r.retained) == 0 {
					return errors.New("proxy inspect or log capture is missing")
				}
				for name, data := range map[string][]byte{"proxy inspect": r.proxyRaw, "proxy stdout": r.proxyOut, "proxy stderr": r.proxyErr, "retained refusal log": r.retained} {
					if err := f.scan(name, data); err != nil {
						return err
					}
				}
			}
		}
		roots = append(roots, p.w.doctor.checkout)
	}
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || !entry.Type().IsRegular() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return errors.New("could not read a provisioning surface file")
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			// A hostile filename is itself provisioning output. Scan it before
			// using it as a diagnostic label.
			if err := f.scan("provisioning filename", []byte(rel)); err != nil {
				return err
			}
			return f.scan("provisioning file "+rel, data)
		}); err != nil {
			return err
		}
	}
	return nil
}
