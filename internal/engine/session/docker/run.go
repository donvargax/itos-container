package docker

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
)

type runConnection struct {
	terminal    terminal.Dimensions
	proxyConfig *container.Config
	proxyRecord port.Record
	*connection
	expected *container.Config
	record   port.Record
}

func (e Engine) OpenRun(ctx context.Context, b port.Binding) (port.RunConnection, port.Outcome) {
	c, failure := e.open(ctx, b, port.EnvironmentMissing)
	if failure != "" {
		return nil, failure
	}
	return &runConnection{connection: c.(*connection)}, ""
}

func (c *runConnection) CreateRun(ctx context.Context, r port.Record, argv []string) (string, port.Outcome) {
	if !r.ValidCredentials(c.binding) || r.Schema == 4 && !r.Credentials.Ready() {
		return "", port.CredentialRefused
	}

	if r.HasEgress() != (c.binding.Egress != (port.EgressGrant{})) {
		return "", port.Refused
	}
	if (r.Schema != 2 && r.Schema != 3 && r.Schema != 4) || !r.ValidAnchor(c.binding) || r.Anchor.Stage != "owned" {
		return "", port.MountRefused
	}
	if !c.pin.Unchanged() {
		return "", port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	networking, address, failure := c.workerNetwork(call, r)
	if failure != "" {
		return "", failure
	}
	image, err := c.api.ImageInspect(call, r.ImageID)
	if !c.pin.Unchanged() {
		return "", port.Refused
	}
	if err != nil || image.Config == nil {
		return "", port.Uncertain
	}
	if image.ID != r.ImageID || len(image.Config.Volumes) != 0 {
		return "", port.Refused
	}
	var env map[string]string
	if json.Unmarshal([]byte(c.binding.Environment), &env) != nil {
		return "", port.Invalid
	}
	if env == nil {
		env = map[string]string{}
	}
	if address != "" {
		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
			env[key] = "http://" + address + ":3128"
		}
		env["NO_PROXY"], env["no_proxy"] = "localhost,127.0.0.1,::1", "localhost,127.0.0.1,::1"
	}
	environment := environment(image.Config.Env, env)
	pids, init := int64(4096), false
	config := &container.Config{
		Tty:   c.terminal.Valid(),
		Image: r.ImageID, User: r.UID + ":" + r.GID, WorkingDir: c.binding.Workspace,
		Cmd: slices.Clone(argv), Entrypoint: []string{}, Env: environment,
		AttachStdin: true, AttachStdout: true, AttachStderr: true, OpenStdin: true, StdinOnce: true,
		Healthcheck: &container.HealthConfig{Test: []string{"NONE"}},
		Labels:      map[string]string{"io.itos-container.intent": r.NameToken(), "io.itos-container.project": r.Project, "io.itos-container.fingerprint": r.Fingerprint},
	}
	host := &container.HostConfig{
		ConsoleSize: [2]uint{uint(c.terminal.Rows), uint(c.terminal.Columns)},
		NetworkMode: "none", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"},
		Resources: container.Resources{PidsLimit: &pids}, RestartPolicy: container.RestartPolicy{Name: "no"},
		IpcMode: "private", CgroupnsMode: "private", Init: &init, LogConfig: container.LogConfig{Type: "none"},
		Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: r.Anchor.Name, Target: "/workspace", VolumeOptions: &mount.VolumeOptions{NoCopy: true}}},
		Tmpfs:  map[string]string{"/tmp": "rw,nosuid,nodev,mode=1777"},
	}
	if r.HasEgress() {
		host.NetworkMode = container.NetworkMode(r.Egress.Internal.Name)
	}
	for _, credential := range r.Credentials.Mounts() {
		host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeBind, Source: credential.Source, Target: credential.Target, ReadOnly: true})
	}
	result, err := c.api.ContainerCreate(call, client.ContainerCreateOptions{Name: r.Name, Config: config, HostConfig: host, NetworkingConfig: networking})
	if err != nil || !c.pin.Unchanged() || !port.HexID(result.ID) {
		return "", port.Uncertain
	}
	c.expected, c.record = config, r
	return result.ID, ""
}

func environment(defaults []string, overlay map[string]string) []string {
	env := map[string]string{}
	for _, entry := range defaults {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	maps.Copy(env, overlay)
	env["HOME"], env["TMPDIR"] = "/tmp", "/tmp"
	result := make([]string, 0, len(env))
	for _, key := range slices.Sorted(maps.Keys(env)) {
		result = append(result, key+"="+env[key])
	}
	return result
}

func (c *runConnection) VerifyRun(ctx context.Context, id string, argv []string) port.Outcome {
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	inspected, err := c.api.ContainerInspect(call, id, client.ContainerInspectOptions{})
	if !c.pin.Unchanged() {
		return port.Refused
	}
	r := inspected.Container
	if err != nil || r.Config == nil || r.HostConfig == nil || r.State == nil || r.NetworkSettings == nil {
		return port.IsolationInspection
	}
	if r.ID != id || r.Image != c.record.ImageID || strings.TrimPrefix(r.Name, "/") != c.record.Name ||
		r.Config.Labels["io.itos-container.intent"] != c.record.NameToken() || r.Config.Labels["io.itos-container.project"] != c.record.Project || r.Config.Labels["io.itos-container.fingerprint"] != c.record.Fingerprint {
		return port.Refused
	}
	if c.expected == nil || !slices.Equal(r.Config.Cmd, argv) || !slices.Equal(r.Config.Cmd, c.expected.Cmd) ||
		len(r.Config.Entrypoint) != 0 || r.Config.User != c.expected.User || r.Config.WorkingDir != c.expected.WorkingDir ||
		!sameEnvironment(r.Config.Env, c.expected.Env) || r.Config.Tty != c.expected.Tty || !r.Config.OpenStdin || !r.Config.StdinOnce ||
		r.HostConfig.ConsoleSize != [2]uint{uint(c.terminal.Rows), uint(c.terminal.Columns)} ||
		len(r.Config.Volumes) != 0 || r.Config.Healthcheck == nil || !slices.Equal(r.Config.Healthcheck.Test, []string{"NONE"}) ||
		r.State.Status != "created" || r.State.Running || r.State.Pid != 0 || (r.State.StartedAt != "" && r.State.StartedAt != "0001-01-01T00:00:00Z") ||
		!safeWorkerHost(r.HostConfig, c.record) || !safeMounts(r.Mounts, c.record.Anchor.Name, c.record.Credentials.Mounts()...) {
		return port.IsolationUnverified
	}
	if c.record.HasEgress() {
		if !egressEndpoints(r.NetworkSettings, map[string]string{c.record.Egress.Internal.Name: c.record.Egress.Internal.ID}) {
			return port.IsolationUnverified
		}
		return ""
	}
	for name := range r.NetworkSettings.Networks {
		if name != "none" {
			return port.IsolationUnverified
		}
	}
	for _, bindings := range r.NetworkSettings.Ports {
		if len(bindings) != 0 {
			return port.IsolationUnverified
		}
	}
	return ""
}

func sameEnvironment(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func safeHost(h *container.HostConfig, root string, credentials ...port.CredentialMount) bool {
	if h.NetworkMode != "none" || !h.ReadonlyRootfs || h.Privileged || h.AutoRemove || h.PublishAllPorts ||
		len(h.CapAdd) != 0 || !slices.Equal(h.CapDrop, []string{"ALL"}) || len(h.GroupAdd) != 0 ||
		h.PidsLimit == nil || *h.PidsLimit != 4096 || h.RestartPolicy.Name != "no" ||
		h.PidMode != "" || h.IpcMode != "private" || h.UTSMode != "" || h.UsernsMode != "" || h.CgroupnsMode != "private" ||
		len(h.Devices) != 0 || len(h.DeviceRequests) != 0 || len(h.DeviceCgroupRules) != 0 || len(h.PortBindings) != 0 ||
		len(h.Binds) != 0 || len(h.VolumesFrom) != 0 || len(h.Links) != 0 || len(h.ExtraHosts) != 0 ||
		len(h.Sysctls) != 0 || h.Init == nil || *h.Init || h.LogConfig.Type != "none" || len(h.Mounts) != 1+len(credentials) ||
		len(h.Tmpfs) != 1 || h.Tmpfs["/tmp"] != "rw,nosuid,nodev,mode=1777" {
		return false
	}
	if len(h.SecurityOpt) != 1 || (h.SecurityOpt[0] != "no-new-privileges" && h.SecurityOpt[0] != "no-new-privileges=true") {
		return false
	}
	seen := map[string]bool{}
	anchor := false
	for _, m := range h.Mounts {
		if safeAnchorMount(m, root) && !anchor {
			anchor = true
			continue
		}
		matched := false
		for _, credential := range credentials {
			if !seen[credential.Kind] && safeCredentialMount(m, credential) {
				seen[credential.Kind] = true
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return anchor && len(seen) == len(credentials)
}

func safeAnchorMount(m mount.Mount, root string) bool {
	return root != "" && m.Type == mount.TypeVolume && m.Source == root && m.Target == "/workspace" && !m.ReadOnly &&
		m.BindOptions == nil && m.VolumeOptions != nil && m.VolumeOptions.NoCopy && m.VolumeOptions.Subpath == "" &&
		m.VolumeOptions.DriverConfig == nil && len(m.VolumeOptions.Labels) == 0
}

func safeMounts(mounts []container.MountPoint, root string, credentials ...port.CredentialMount) bool {
	seen := map[string]bool{}
	bind, tmp := false, false
	for _, m := range mounts {
		switch {
		case m.Type == mount.TypeVolume && m.Name == root && m.Driver == "local" && m.Destination == "/workspace" && m.RW && (m.Propagation == "" || m.Propagation == mount.PropagationRPrivate) && !bind:
			bind = true
		case m.Type == mount.TypeTmpfs && m.Destination == "/tmp" && !tmp:
			tmp = true
		default:
			matched := false
			for _, credential := range credentials {
				if !seen[credential.Kind] && m.Type == mount.TypeBind && m.Source == credential.Source && m.Destination == credential.Target && !m.RW && m.Name == "" && m.Driver == "" && (m.Propagation == "" || m.Propagation == mount.PropagationRPrivate) {
					seen[credential.Kind] = true
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	// Docker represents HostConfig.Tmpfs separately on some API versions. The
	// exact tmpfs options are always verified in HostConfig before start.
	return bind && len(seen) == len(credentials)
}

type attachment struct {
	response client.ContainerAttachResult
	done     chan struct{}
	failures chan port.Outcome
	mu       sync.Mutex
	failure  port.Outcome
}

// terminalDetachKeys pins the escape sequence the daemon applies to a TTY
// attach's input. The daemon cannot disable that handling: an absent value
// keeps Docker's default ctrl-p,ctrl-q, which it swallows as a detach while
// dropping the attach, so those workload bytes would never arrive. This exact
// byte sequence is the one input run cannot forward; it deliberately begins
// with a byte no workload control-key exchange sends, and everything else,
// Docker's default included, reaches the worker unchanged.
const terminalDetachKeys = "ctrl-^,ctrl-_,ctrl-^,ctrl-_,ctrl-^,ctrl-_,ctrl-^,ctrl-_"

func (c *runConnection) Attach(ctx context.Context, id string, streams port.Streams) (port.Attachment, port.Outcome) {
	call, cancel := c.call(ctx)
	defer cancel()
	options := client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true}
	if c.terminal.Valid() {
		options.DetachKeys = terminalDetachKeys
	}
	response, err := c.api.ContainerAttach(call, id, options)
	if err != nil {
		return nil, port.ExecutionFailed
	}
	a := &attachment{response: response, done: make(chan struct{}), failures: make(chan port.Outcome, 1)}
	go func() {
		if streams.In != nil {
			if _, err := io.Copy(response.Conn, stdinSource{streams.In, a.fail}); err != nil {
				a.notify(port.StdinClosed)
			}
		}
		if err := response.CloseWrite(); err != nil {
			a.notify(port.StdinClosed)
		}
	}()
	go func() {
		out, errout := io.Writer(io.Discard), io.Writer(io.Discard)
		if streams.Out != nil {
			out = streams.Out
		}
		if streams.Err != nil {
			errout = streams.Err
		}
		var err error
		if c.terminal.Valid() {
			_, err = io.Copy(out, response.Reader)
		} else {
			_, err = stdcopy.StdCopy(out, errout, response.Reader)
		}
		if err != nil {
			a.fail()
		}
		close(a.done)
	}()
	return a, ""
}

// stdinSource distinguishes a host input read error from the worker closing its
// input connection. Wrapping the reader keeps io.Copy's fast paths from hiding
// source errors, including reads that return both bytes and an error.
type stdinSource struct {
	port.Reader
	fail func()
}

func (s stdinSource) Read(p []byte) (int, error) {
	n, err := s.Reader.Read(p)
	if err != nil && err != io.EOF {
		s.fail()
	}
	return n, err
}

func (a *attachment) fail() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failure = port.ExecutionFailed
	a.notify(port.ExecutionFailed)
}
func (a *attachment) notify(failure port.Outcome) {
	select {
	case a.failures <- failure:
	default:
	}
}
func (a *attachment) Failures() <-chan port.Outcome { return a.failures }
func (a *attachment) Drain(ctx context.Context) port.Outcome {
	for {
		select {
		case <-a.done:
			a.mu.Lock()
			defer a.mu.Unlock()
			return a.failure
		case failure := <-a.failures:
			if failure != port.StdinClosed {
				return failure
			}
		case <-ctx.Done():
			return port.ExecutionFailed
		}
	}
}
func (a *attachment) Close() { a.response.Close() }
func (c *runConnection) Start(ctx context.Context, id string) port.Outcome {
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, err := c.api.ContainerStart(call, id, client.ContainerStartOptions{})
	if err != nil {
		return port.ExecutionFailed
	}
	return ""
}
func (c *runConnection) Wait(ctx context.Context, id string) (int, port.Outcome) {
	if !c.pin.Unchanged() {
		return 0, port.Refused
	}
	// Bound the HTTP handshake, while the acknowledged wait stream can live for
	// the whole foreground workload. This is not a ten-second workload timeout.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(budget.CallDuration, cancel)
	result := c.api.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	timer.Stop()
	select {
	case status := <-result.Result:
		if status.Error != nil || status.StatusCode < 0 || status.StatusCode > 255 || !c.pin.Unchanged() {
			return 0, port.ExecutionFailed
		}
		return int(status.StatusCode), ""
	case <-result.Error:
		if ctx.Err() != nil {
			return 0, port.ExecutionFailed
		}
		return 0, port.EngineLost
	case <-ctx.Done():
		return 0, port.ExecutionFailed
	}
}
func (c *runConnection) RemoveRun(ctx context.Context, id string) {
	if !c.pin.Unchanged() {
		return
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, _ = c.api.ContainerRemove(call, id, client.ContainerRemoveOptions{Force: true})
}

func (c *runConnection) Signal(ctx context.Context, id, signal string) port.Outcome {
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, err := c.api.ContainerKill(call, id, client.ContainerKillOptions{Signal: signal})
	if err != nil {
		return port.ExecutionFailed
	}
	return ""
}
