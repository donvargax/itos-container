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

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type runConnection struct {
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
	if !c.pin.Unchanged() {
		return "", port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
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
	environment := environment(image.Config.Env, env)
	pids, init := int64(4096), false
	config := &container.Config{
		Image: r.ImageID, User: r.UID + ":" + r.GID, WorkingDir: c.binding.Workspace,
		Cmd: slices.Clone(argv), Entrypoint: []string{}, Env: environment,
		AttachStdin: true, AttachStdout: true, AttachStderr: true, OpenStdin: true, StdinOnce: true,
		Healthcheck: &container.HealthConfig{Test: []string{"NONE"}},
		Labels:      map[string]string{"io.itos-container.intent": r.NameToken(), "io.itos-container.project": r.Project, "io.itos-container.fingerprint": r.Fingerprint},
	}
	host := &container.HostConfig{
		NetworkMode: "none", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"},
		Resources: container.Resources{PidsLimit: &pids}, RestartPolicy: container.RestartPolicy{Name: "no"},
		IpcMode: "private", CgroupnsMode: "private", Init: &init, LogConfig: container.LogConfig{Type: "none"},
		Mounts: []mount.Mount{{Type: mount.TypeBind, Source: c.binding.Checkout.Root, Target: "/workspace", BindOptions: &mount.BindOptions{Propagation: mount.PropagationRPrivate, NonRecursive: true}}},
		Tmpfs:  map[string]string{"/tmp": "rw,nosuid,nodev,mode=1777"},
	}
	result, err := c.api.ContainerCreate(call, client.ContainerCreateOptions{Name: r.Name, Config: config, HostConfig: host})
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
		!sameEnvironment(r.Config.Env, c.expected.Env) || r.Config.Tty || !r.Config.OpenStdin || !r.Config.StdinOnce ||
		len(r.Config.Volumes) != 0 || r.Config.Healthcheck == nil || !slices.Equal(r.Config.Healthcheck.Test, []string{"NONE"}) ||
		r.State.Status != "created" || r.State.Running || r.State.Pid != 0 || (r.State.StartedAt != "" && r.State.StartedAt != "0001-01-01T00:00:00Z") ||
		!safeHost(r.HostConfig, c.binding.Checkout.Root) || !safeMounts(r.Mounts, c.binding.Checkout.Root) {
		return port.IsolationUnverified
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

func safeHost(h *container.HostConfig, root string) bool {
	if h.NetworkMode != "none" || !h.ReadonlyRootfs || h.Privileged || h.AutoRemove || h.PublishAllPorts ||
		len(h.CapAdd) != 0 || !slices.Equal(h.CapDrop, []string{"ALL"}) || len(h.GroupAdd) != 0 ||
		h.PidsLimit == nil || *h.PidsLimit != 4096 || h.RestartPolicy.Name != "no" ||
		h.PidMode != "" || h.IpcMode != "private" || h.UTSMode != "" || h.UsernsMode != "" || h.CgroupnsMode != "private" ||
		len(h.Devices) != 0 || len(h.DeviceRequests) != 0 || len(h.DeviceCgroupRules) != 0 || len(h.PortBindings) != 0 ||
		len(h.Binds) != 0 || len(h.VolumesFrom) != 0 || len(h.Links) != 0 || len(h.ExtraHosts) != 0 ||
		len(h.Sysctls) != 0 || h.Init == nil || *h.Init || h.LogConfig.Type != "none" || len(h.Mounts) != 1 ||
		len(h.Tmpfs) != 1 || h.Tmpfs["/tmp"] != "rw,nosuid,nodev,mode=1777" {
		return false
	}
	if len(h.SecurityOpt) != 1 || (h.SecurityOpt[0] != "no-new-privileges" && h.SecurityOpt[0] != "no-new-privileges=true") {
		return false
	}
	m := h.Mounts[0]
	return m.Type == mount.TypeBind && m.Source == root && m.Target == "/workspace" && !m.ReadOnly &&
		m.BindOptions != nil && m.BindOptions.Propagation == mount.PropagationRPrivate && m.BindOptions.NonRecursive && !m.BindOptions.CreateMountpoint
}

func safeMounts(mounts []container.MountPoint, root string) bool {
	bind, tmp := false, false
	for _, m := range mounts {
		switch {
		case m.Type == mount.TypeBind && m.Source == root && m.Destination == "/workspace" && m.RW && m.Propagation == mount.PropagationRPrivate && !bind:
			bind = true
		case m.Type == mount.TypeTmpfs && m.Destination == "/tmp" && !tmp:
			tmp = true
		default:
			return false
		}
	}
	// Docker represents HostConfig.Tmpfs separately on some API versions. The
	// exact tmpfs options are always verified in HostConfig before start.
	return bind
}

type attachment struct {
	response client.ContainerAttachResult
	done     chan struct{}
	failures chan port.Outcome
	mu       sync.Mutex
	failure  port.Outcome
}

func (c *runConnection) Attach(ctx context.Context, id string, streams port.Streams) (port.Attachment, port.Outcome) {
	call, cancel := c.call(ctx)
	defer cancel()
	response, err := c.api.ContainerAttach(call, id, client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true})
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
		_, err := stdcopy.StdCopy(out, errout, response.Reader)
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
	timer := time.AfterFunc(10*time.Second, cancel)
	result := c.api.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	timer.Stop()
	select {
	case status := <-result.Result:
		if status.Error != nil || status.StatusCode < 0 || status.StatusCode > 255 || !c.pin.Unchanged() {
			return 0, port.ExecutionFailed
		}
		return int(status.StatusCode), ""
	case <-result.Error:
		return 0, port.ExecutionFailed
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
