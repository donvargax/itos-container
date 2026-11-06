package features

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type isolationRoute struct{ Name, Protocol, Address, Answer string }
type isolationAttempt struct {
	Name, Protocol, Address, Nonce, Result, Error string
	Attempted                                     bool
}
type isolationReport struct {
	Kind, Nonce, Phase string
	Values             map[string]string
	Attempts           []isolationAttempt
	Error              string
}
type isolationConfig struct {
	Nonce, Previous, Mode string
	Status                int
	Files, Sockets        map[string]string
	HostPID               int
	HostStart             string
	Routes                []isolationRoute
	Addresses, Resolvers  []string
	CA, Cert, Key         string
	Port                  int
}
type isolationRun struct {
	config           isolationConfig
	exit             int
	stdout, stderr   []byte
	reports          []isolationReport
	requests         []offlineRequest
	id               string
	record           sessionRecord
	sessionID        string
	registration     sessionRecord
	snapshot         sessionRecord
	input            sessionRecord
	expected         map[string]any
	absent           bool
	running, stopped map[string]any
	descendantPID    int
	descendantStart  string
	err              error
}
type isolationControl struct {
	id, name, image string
	labels          map[string]string
}
type isolationWorld struct {
	w                                     *world
	cli                                   *client.Client
	mode                                  string
	config                                isolationConfig
	runs                                  []*isolationRun
	controls                              []*isolationControl
	cleanup                               []func() error
	networkID, networkName                string
	networkLabels                         map[string]string
	control                               *isolationControl
	privilegedControl                     *isolationControl
	controlDir                            string
	before, after                         map[string]string
	resourcesBefore, resourcesAfter       map[string]string
	controlsBefore, controlsAfter         []isolationReport
	controlError                          error
	removeFault                           bool
	nestedBefore, nestedAfter             bool
	setuidBefore, setuidAfter             string
	privilegedBefore, privilegedAfter     bool
	rootBefore, rootAfter                 bool
	hostBefore, hostAfter                 map[string]string
	groups                                []int
	controlBeatsBefore, controlBeatsAfter int
	mu                                    sync.Mutex
	receipts                              []string
}

func isolationNonce() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (w *world) isolationFixture(mode string) error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	if err := w.offlineFixture("isolation"); err != nil {
		return err
	}
	s := &isolationWorld{w: w, mode: mode, config: isolationConfig{Nonce: isolationNonce(), Mode: mode, Files: map[string]string{}, Sockets: map[string]string{}}}
	w.offline.isolation = s
	cli, err := client.New(client.WithHost("unix:///var/run/docker.sock"))
	if err != nil {
		return err
	}
	s.cli = cli
	if err = w.buildIsolationProbe(); err != nil {
		return err
	}
	if err = s.prepare(); err != nil {
		return err
	}
	return s.checkpoint()
}

func (w *world) buildIsolationProbe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base := filepath.Join(w.root, "features", "testdata", "slice-14")
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", filepath.Join(w.doctor.checkout, "slice-14-probe"), filepath.Join(base, "probe.go"), filepath.Join(base, "network.go"), filepath.Join(base, "dns.go"), filepath.Join(base, "dns_wire.go"))
	cmd.Dir, cmd.Env = w.root, sessionChildEnvironment(os.Environ())
	for k, v := range map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64", "GOFLAGS": "", "GOWORK": "off"} {
		cmd.Env = setEnv(cmd.Env, k, v)
	}
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compile slice-14 trusted probe: %w\n%s", err, b)
	}
	return nil
}

func (s *isolationWorld) controlCreate(dir string, argv []string, user, networkID string, aliases []string, capabilities []string, nnp bool, configure ...func(*container.HostConfig)) (*isolationControl, error) {
	c := &isolationControl{name: "slice-14-control-" + isolationNonce(), image: s.w.doctor.imageID, labels: map[string]string{"slice-14.fixture": isolationNonce()}}
	s.controls = append(s.controls, c) // retain intent even if create's response is lost
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// The root controls must traverse their runner-owned 0700 fixture directory.
	// This capability belongs only to trusted controls, never the subject.
	if user == "0:0" {
		capabilities = append(append([]string{}, capabilities...), "DAC_OVERRIDE")
	}
	host := &container.HostConfig{NetworkMode: container.NetworkMode(networkID), ReadonlyRootfs: true, CapDrop: []string{"ALL"}, CapAdd: capabilities, Tmpfs: map[string]string{"/tmp": "rw,nosuid,nodev,mode=1777"}, LogConfig: container.LogConfig{Type: "local"}, Mounts: []mount.Mount{{Type: mount.TypeBind, Source: dir, Target: "/workspace", BindOptions: &mount.BindOptions{NonRecursive: true, Propagation: mount.PropagationRPrivate}}}}
	if nnp {
		host.SecurityOpt = []string{"no-new-privileges"}
	}
	for _, apply := range configure {
		apply(host)
	}
	var netcfg *network.NetworkingConfig
	if networkID != "none" {
		netcfg = &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{networkID: {Aliases: aliases, IPAMConfig: &network.EndpointIPAMConfig{IPv4Address: netip.MustParseAddr("172.30.14.2"), IPv6Address: netip.MustParseAddr("fd14:14::2")}}}}
	}
	created, err := s.cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: c.name, Config: &container.Config{Image: c.image, User: user, Entrypoint: []string{}, Cmd: argv, Labels: c.labels, Env: []string{"HOME=/tmp", "TMPDIR=/tmp"}, Healthcheck: &container.HealthConfig{Test: []string{"NONE"}}}, HostConfig: host, NetworkingConfig: netcfg})
	if err != nil {
		return c, err
	}
	c.id = created.ID
	if _, err = s.controlInspect(c); err != nil {
		return c, err
	}
	_, err = s.cli.ContainerStart(ctx, c.id, client.ContainerStartOptions{})
	return c, err
}

func (s *isolationWorld) controlInspect(c *isolationControl) (container.InspectResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	target := c.id
	if target == "" {
		target = c.name
	}
	r, err := s.cli.ContainerInspect(ctx, target, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, err
	}
	v := r.Container
	if !offlineHex(v.ID) || v.Name != "/"+c.name || v.Image != c.image || v.Config == nil || !reflect.DeepEqual(v.Config.Labels, c.labels) {
		return v, errors.New("control identity changed")
	}
	c.id = v.ID
	return v, nil
}

func (s *isolationWorld) controlLogs(c *isolationControl) ([]isolationReport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reader, err := s.cli.ContainerLogs(ctx, c.id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	var out, errout bytes.Buffer
	if _, err = stdcopy.StdCopy(&out, &errout, io.LimitReader(reader, 4<<20)); err != nil {
		return nil, err
	}
	if errout.Len() != 0 {
		return nil, errors.New("trusted control wrote unexpected stderr")
	}
	return isolationReports(out.Bytes())
}

var errIsolationNoReports = errors.New("probe did not produce a report")

func isolationReports(b []byte) ([]isolationReport, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	var result []isolationReport
	for {
		var r isolationReport
		err := d.Decode(&r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("probe report is incomplete or not framed JSON")
		}
		if r.Kind == "error" {
			return nil, fmt.Errorf("probe operation failed: %s", r.Error)
		}
		result = append(result, r)
	}
	if len(result) == 0 {
		return nil, errIsolationNoReports
	}
	return result, nil
}

func (s *isolationWorld) controlWait(c *isolationControl) ([]isolationReport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := s.cli.ContainerWait(ctx, c.id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-r.Error:
		return nil, err
	case v := <-r.Result:
		if v.StatusCode != 0 {
			return nil, fmt.Errorf("trusted control exit %d", v.StatusCode)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.controlLogs(c)
}

func (s *isolationWorld) controlExec(c *isolationControl, user string, argv ...string) ([]isolationReport, error) {
	if _, err := s.controlInspect(c); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := s.cli.ExecCreate(ctx, c.id, client.ExecCreateOptions{User: user, Cmd: argv, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return nil, err
	}
	a, err := s.cli.ExecAttach(ctx, r.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, err
	}
	defer a.Close()
	_ = a.Conn.SetDeadline(time.Now().Add(25 * time.Second))
	var out, errout bytes.Buffer
	if _, err = stdcopy.StdCopy(&out, &errout, a.Reader); err != nil {
		return nil, err
	}
	observed, err := s.cli.ExecInspect(ctx, r.ID, client.ExecInspectOptions{})
	if err != nil {
		return nil, err
	}
	if observed.Running || observed.ExitCode != 0 || errout.Len() != 0 {
		return nil, errors.New("trusted control exec did not finish successfully")
	}
	return isolationReports(out.Bytes())
}

func (s *isolationWorld) close() error {
	var result error
	if s.cli != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for i := len(s.controls) - 1; i >= 0; i-- {
			c := s.controls[i]
			if _, err := s.controlInspect(c); err != nil {
				result = errors.Join(result, err)
				continue
			}
			_, err := s.cli.ContainerRemove(ctx, c.id, client.ContainerRemoveOptions{Force: true})
			result = errors.Join(result, err)
		}
		if s.networkName != "" {
			target := s.networkID
			if target == "" {
				target = s.networkName
			}
			n, err := s.cli.NetworkInspect(ctx, target, client.NetworkInspectOptions{})
			if err == nil && offlineHex(n.Network.ID) && n.Network.Name == s.networkName && reflect.DeepEqual(n.Network.Labels, s.networkLabels) {
				_, err = s.cli.NetworkRemove(ctx, n.Network.ID, client.NetworkRemoveOptions{})
			} else {
				err = errors.Join(err, errors.New("refuse mismatched control network cleanup"))
			}
			result = errors.Join(result, err)
		}
		result = errors.Join(result, s.cli.Close())
	}
	for i := len(s.cleanup) - 1; i >= 0; i-- {
		result = errors.Join(result, s.cleanup[i]())
	}
	return result
}

func (s *isolationWorld) checkpoint() error {
	var err error
	s.before, err = s.protected()
	if err != nil {
		return err
	}
	s.resourcesBefore, err = s.w.doctor.real.snapshotResources()
	return err
}

func (s *isolationWorld) protected() (map[string]string, error) {
	r, err := s.w.offlineProtected()
	if err != nil {
		return nil, err
	}
	for path := range r {
		for _, run := range s.runs {
			if path == filepath.Join(s.w.doctor.checkout, "slice-14-edit-"+run.config.Nonce) || path == filepath.Join(s.w.doctor.checkout, "slice-14-run-"+run.config.Nonce+".json") {
				delete(r, path)
			}
		}
		if _, ok := r[path]; ok {
			info, err := os.Lstat(path)
			if err != nil {
				return nil, err
			}
			stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
			r[path] += fmt.Sprint(":gid=", stat.FieldByName("Gid"))
		}
	}
	return r, nil
}

func (a *offlineAPI) removalFault(r offlineRequest) bool {
	s := a.w.offline.isolation
	if s == nil || !s.removeFault || r.Method != "DELETE" {
		return false
	}
	for _, previous := range a.requests {
		if previous.Method == "POST" && previous.Path == r.Path+"/wait" && previous.Status == 200 && previous.Observation["StatusCode"] != nil {
			return true
		}
	}
	return false
}
