package features

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const credentialRunTarget = "/run/itos-container/credentials/claude-token"

type credentialRunPlan struct {
	Nonce, Target string
	Forbidden     []string
	Child         json.RawMessage
}

type credentialRunFixture struct {
	root, state, account, control, staging, detectors string
	projects                                          []*credentialRunProject
	journal                                           *credentialRunJournal
	cli                                               *client.Client
	needles                                           []credentialRunNeedle
	initialShm                                        map[string]bool
	originalResources                                 map[string]string
	egress                                            *egressNative
	endpoint                                          *egressEndpoint
	verified, controlled                              bool
	rotation                                          string
	rotated                                           []byte
	rotatedInfo                                       os.FileInfo
	sourceAfterRotation                               string
	later                                             *credentialRunProcess
}

type credentialRunProject struct {
	checkoutEditRejected                    bool
	w                                       *world
	observer                                *credentialRunCheckout
	source, sibling                         string
	originalSource, originalSibling         []byte
	sourceInfo, siblingInfo                 os.FileInfo
	sourceDir, control, staging             string
	checkoutBefore                          map[string]string
	plans                                   []credentialRunPlan
	runs                                    []*credentialRunProcess
	doctor, plainDoctor, approve, stop      *homeNativeResult
	approvalBeforeTools, approvalAfterTools map[string]string
}

func (w *world) openCredentialRun() (*credentialRunFixture, error) {
	if err := doctorRealGuard(); err != nil {
		return nil, err
	}
	if os.Geteuid() <= 0 || strconv.Itoa(os.Geteuid()) != os.Getenv("ITOS_CONTAINER_ENGINE_FIXTURE_UID") ||
		strconv.Itoa(os.Getegid()) != os.Getenv("ITOS_CONTAINER_ENGINE_FIXTURE_GID") {
		return nil, errors.New("credential run fixture requires the non-root cached image UID and GID")
	}
	root, err := os.MkdirTemp("/var/tmp", "credrun-")
	if err != nil {
		return nil, err
	}
	f := &credentialRunFixture{
		root: root, state: filepath.Join(root, "state"), account: filepath.Join(root, "account"),
		control: filepath.Join(root, "control"), staging: filepath.Join(root, "policy-staging"), detectors: filepath.Join(root, "detector-controls"),
	}
	w.credentialRun = f
	for _, path := range []string{f.state, f.fixedState(), f.control, f.staging, f.detectors} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
	}
	if f.initialShm, err = credentialRunShmEntries(); err != nil {
		return nil, err
	}
	if f.journal, err = newCredentialRunJournal(filepath.Join(root, "engine.sock")); err != nil {
		return nil, err
	}
	f.cli, err = client.New(client.WithHost("unix:///var/run/docker.sock"))
	return f, err
}

func (f *credentialRunFixture) fixedState() string {
	return filepath.Join(f.account, ".local/state/itos-container")
}

func (w *world) credentialRunProject(network string, grant bool) (*credentialRunProject, error) {
	f := w.credentialRun
	name := fmt.Sprintf("project-%d", len(f.projects)+1)
	p := &credentialRunProject{
		w:         &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(f.root, name)},
		sourceDir: filepath.Join(f.root, name, "sources"), control: filepath.Join(f.control, name), staging: filepath.Join(f.staging, name),
	}
	f.projects = append(f.projects, p)
	for _, path := range []string{p.sourceDir, p.control, p.staging} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
	}
	if err := p.w.manifestCheckout(); err != nil {
		return nil, err
	}
	if err := p.w.doctorRealCheckout(false); err != nil {
		return nil, err
	}
	d := p.w.doctor
	d.engine = newDoctorAPI(d.image)
	d.engine.info["ID"], d.engine.image["Id"] = d.engineID, d.imageID
	d.env["DOCKER_HOST"] = "unix://" + filepath.Join(f.root, "engine.sock")
	d.env["XDG_STATE_HOME"], d.env["SLICE_21_ACCOUNT_HOME"] = f.state, f.account
	d.env["ITOS_ROOT"] = d.checkout
	p.w.approval = &approvalWorld{uid: strconv.Itoa(os.Geteuid()), gid: strconv.Itoa(os.Getegid()), policyVersion: 3}
	p.w.credentials = &credentialWorld{paths: map[string]string{}}
	p.w.credentialApproval = &credentialApprovalWorld{}
	if grant {
		p.source = filepath.Join(p.sourceDir, "claude-token")
		var err error
		p.originalSource, err = f.writeMarker(p.source, 0600)
		if err != nil {
			return nil, err
		}
		p.sourceInfo, err = os.Lstat(p.source)
		if err != nil {
			return nil, err
		}
		p.w.credentials.paths["claude-token"] = p.source
		p.w.credentialApproval.grants = []credentialGrant{{Kind: "claude-token", Source: p.source, Target: credentialRunTarget}}
	}
	if network == "allowlist" {
		p.w.approval.allow = []string{"credential-ci.example"}
	}
	if err := os.WriteFile(filepath.Join(d.host, "slice-30-read-control"), []byte("read observer control"), 0600); err != nil {
		return nil, err
	}
	probe := func(output string) error {
		return egressBuild(w.root, filepath.Join("features", "testdata", "credentials-run", "probe"), output)
	}
	if err := p.w.installNativeProbe("credrun-probe", probe); err != nil {
		return nil, err
	}
	if err := verifyCredentialRunFilesystem([]string{p.sourceDir, d.checkout, f.state, f.fixedState()}); err != nil {
		return nil, err
	}
	if p.source != "" {
		if err := verifyCredentialRunFilesystem([]string{p.source}); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(d.checkout, "itos-container.yaml"), p.policy(), 0600); err != nil {
		return nil, err
	}
	if err := p.seed("4"); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	owned, err := p.engine(f.cli).owned(ctx)
	if err != nil || len(owned) != 0 {
		return nil, errors.New("credential run project already owns resources or inventory is unavailable")
	}
	return p, nil
}

func (p *credentialRunProject) policy() []byte {
	network := map[string]any{"mode": "offline"}
	if p.w.approval.allow != nil {
		network = map[string]any{"mode": "allowlist", "allow": p.w.approval.allow}
	}
	policy := map[string]any{"version": 3, "manifest": ".devcontainer/devcontainer.json", "network": network}
	if len(p.w.credentialApproval.grants) != 0 {
		grant := p.w.credentialApproval.grants[0]
		policy["credentials"] = []map[string]string{{"kind": grant.Kind, "source": grant.Source}}
	}
	return approvalJSON(policy)
}

func (p *credentialRunProject) seed(generation string) error {
	return p.w.approvalSeed(generation, false) // Also seeds the fixed control-location catalog.
}

func (p *credentialRunProject) engine(cli *client.Client) *egressNative {
	return &egressNative{w: p.w, cli: cli}
}

func (p *credentialRunProject) plan(forbidden []string, child json.RawMessage) error {
	plan := credentialRunPlan{Nonce: egressNonce(), Target: credentialRunTarget, Forbidden: forbidden, Child: child}
	p.plans = append(p.plans, plan)
	path := filepath.Join(p.w.doctor.checkout, credentialPlanName(len(p.plans)))
	return os.WriteFile(path, approvalJSON(plan), 0600)
}

func credentialPlanName(index int) string {
	return fmt.Sprintf("credrun-plan-%d.json", index)
}

func (f *credentialRunFixture) ready() error {
	var err error
	f.originalResources, err = f.projects[0].w.doctor.real.snapshotResources()
	f.verified = err == nil
	return err
}

func (f *credentialRunFixture) setupEgress(p *credentialRunProject) (json.RawMessage, error) {
	n := &egressNative{w: p.w, cli: f.cli, nonce: egressNonce(), names: p.w.approval.allow, networks: map[string]*egressFixtureNetwork{}, endpoints: map[string]*egressEndpoint{}}
	f.egress = n
	name := n.names[0]
	endpoint, err := n.endpoint(name, "public", []string{name}, "")
	if err != nil {
		return nil, err
	}
	f.endpoint = endpoint
	n.control(endpoint, name)
	if err := errors.Join(endpoint.controls...); err != nil {
		return nil, err
	}
	if err := p.w.installNativeProbe("slice-27-probe", func(output string) error {
		return egressBuild(p.w.root, filepath.Join("features", "testdata", "slice-27", "probe"), output)
	}); err != nil {
		return nil, err
	}
	steps := []egressStep{
		{Op: "tls", Mode: "connect", Name: name, Nonce: "worker-" + egressNonce()},
		{Op: "raw", Method: "CONNECT", Target: "refused.example:443"},
	}
	return approvalJSON(map[string]any{"Steps": steps, "Exit": 0}), nil
}

func (f *credentialRunFixture) positiveProbe(p *credentialRunProject) (retErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := filepath.Join(f.root, "probe-control")
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	target, denied := filepath.Join(root, "readable"), filepath.Join(root, "denied")
	body, err := f.writeMarker(target, 0400)
	if err != nil {
		return err
	}
	if err := os.WriteFile(denied, []byte("denied control"), 0000); err != nil {
		return err
	}
	plan := credentialRunPlan{Nonce: egressNonce(), Target: credentialRunTarget, Forbidden: []string{"/credrun-denied", "/credrun-absent"}}
	planFile := filepath.Join(root, "plan.json")
	if err := os.WriteFile(planFile, approvalJSON(plan), 0600); err != nil {
		return err
	}
	bind := func(source, target string) mount.Mount {
		return mount.Mount{Type: mount.TypeBind, Source: source, Target: target, ReadOnly: true}
	}
	created, err := f.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: "credrun-control-" + plan.Nonce,
		Config: &container.Config{
			Image: p.w.doctor.imageID, User: p.w.approval.uid + ":" + p.w.approval.gid,
			Entrypoint: []string{}, Cmd: []string{"/credrun-probe", "/credrun-plan.json"},
			OpenStdin: true, StdinOnce: true, AttachStdin: true, AttachStdout: true, AttachStderr: true,
			Labels: map[string]string{"credrun.control": plan.Nonce}, Healthcheck: &container.HealthConfig{Test: []string{"NONE"}},
		},
		HostConfig: &container.HostConfig{
			NetworkMode: "none", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"},
			Mounts: []mount.Mount{
				bind(filepath.Join(p.w.doctor.checkout, "credrun-probe"), "/credrun-probe"), bind(planFile, "/credrun-plan.json"),
				bind(target, credentialRunTarget), bind(denied, "/credrun-denied"),
			},
		},
	})
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := f.cli.ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true})
		retErr = errors.Join(retErr, err)
	}()
	attach, err := f.cli.ContainerAttach(ctx, created.ID, client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true})
	if err != nil {
		return err
	}
	defer attach.Close()
	if err := attach.Conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return err
	}
	if _, err := f.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return err
	}
	if _, err := io.WriteString(attach.Conn, "exit 0\n"); err != nil {
		return err
	}
	var output, diagnostic bytes.Buffer
	if _, err := stdcopy.StdCopy(&output, &diagnostic, io.LimitReader(attach.Reader, 1<<20)); err != nil {
		return err
	}
	report, ok := credentialReport(output.Bytes(), plan.Nonce, "first")
	final, finished := credentialReport(output.Bytes(), plan.Nonce, "final")
	if !ok || !finished || !final.hasFile(credentialRunHash(body)) || !report.hasFile(credentialRunHash(body)) ||
		report.UID != os.Geteuid() || report.GID != os.Getegid() || !report.writesDenied() ||
		report.Forbidden["/credrun-denied"].Kind != "denied" || report.Forbidden["/credrun-absent"].Kind != "absent" || diagnostic.Len() != 0 {
		return errors.New("credential hash probe did not distinguish the approved control file from denied and absent paths")
	}
	return nil
}

func credentialRunHash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

type credentialRunNeedle struct {
	name string
	data []byte
}

func (f *credentialRunFixture) marker() ([]byte, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	marker := fmt.Appendf(nil, "itos-synthetic-credential-%x", random)
	index := len(f.needles)/4 + 1
	f.needles = append(f.needles, credentialRunNeedle{fmt.Sprintf("marker-%d-raw", index), bytes.Clone(marker)})
	for offset := range 3 {
		encoded := base64.StdEncoding.EncodeToString(append(bytes.Repeat([]byte("x"), offset), marker...))
		f.needles = append(f.needles, credentialRunNeedle{fmt.Sprintf("marker-%d-base64-%d", index, offset), []byte(encoded[4 : len(encoded)-4])})
	}
	return append(marker, '\n', '\n'), nil
}

func (f *credentialRunFixture) writeMarker(path string, mode fs.FileMode) ([]byte, error) {
	marker, err := f.marker()
	if err != nil {
		return nil, err
	}
	return marker, os.WriteFile(path, marker, mode)
}

func (f *credentialRunFixture) hits(data []byte) []string {
	var hits []string
	for _, needle := range f.needles {
		if bytes.Contains(data, needle.data) {
			hits = append(hits, needle.name)
		}
	}
	return hits
}

func (f *credentialRunFixture) positiveDetectors() error {
	for _, needle := range f.needles {
		path := filepath.Join(f.detectors, needle.name)
		body := bytes.Join([][]byte{[]byte("before:"), needle.data, []byte(":after")}, nil)
		if err := os.WriteFile(path, body, 0600); err != nil {
			return err
		}
		observed, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !slices.Contains(f.hits(observed), needle.name) {
			return fmt.Errorf("missing detector control %s", needle.name)
		}
	}
	f.controlled = true
	return nil
}

func (f *credentialRunFixture) arm() error {
	for _, p := range f.projects {
		if p.observer == nil {
			var err error
			p.checkoutBefore, err = doctorTree(p.w.doctor.checkout)
			if err != nil {
				return err
			}
			p.observer, err = armCredentialRunCheckout(p.w.doctor.checkout, p.control, p.staging)
			if err != nil {
				return err
			}
		}
		if err := p.observer.prove(); err != nil {
			return err
		}
	}
	return nil
}

func credentialRunShmEntries() (map[string]bool, error) {
	result := map[string]bool{}
	entries, err := os.ReadDir("/dev/shm")
	for _, item := range entries {
		if credentialStageName(item.Name()) {
			result[item.Name()] = true
		}
	}
	return result, err
}

func (f *credentialRunFixture) newShmEntries() (map[string]bool, error) {
	entries, err := credentialRunShmEntries()
	maps.DeleteFunc(entries, func(name string, _ bool) bool { return f.initialShm[name] })
	return entries, err
}

func (w *world) closeCredentialRun() error {
	f := w.credentialRun
	if f == nil {
		return nil
	}
	var result error
	for _, p := range f.projects {
		for _, run := range p.runs {
			run.cancel()
			<-run.done
		}
		if p.observer != nil {
			result = errors.Join(result, p.observer.close())
		}
		if err := f.cleanupProject(p); err != nil {
			result = errors.Join(result, err)
		}
		p.w.closeDoctor()
	}
	if f.egress != nil {
		f.egress.w.egressFixture().native = f.egress
		result = errors.Join(result, f.egress.w.closeEgressNative())
	} else if f.cli != nil {
		result = errors.Join(result, f.cli.Close())
	}
	if f.journal != nil {
		result = errors.Join(result, f.journal.close())
	}
	if result != nil {
		return result
	}
	return os.RemoveAll(f.root)
}
