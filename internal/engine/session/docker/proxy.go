package docker

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"path"
	"slices"
	"strings"
)

func (c *runConnection) CreateProxy(ctx context.Context, r port.Record) (string, port.Outcome) {
	if !c.pin.Unchanged() {
		return "", port.Refused
	}
	// The bind source belongs to the Linux daemon's filesystem, even when the
	// SDK client (or its HTTP fixture) runs on a different host OS.
	if r.Schema != 3 || !r.ValidEgress(c.binding) || r.Stage != "reserved" || r.Egress.Internal.Stage != "owned" || r.Egress.Network.Stage != "owned" || r.Egress.Proxy.Stage != "intent" || !path.IsAbs(c.executable) {
		return "", port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	image, err := c.api.ImageInspect(call, c.binding.Egress.ImageID)
	if !c.pin.Unchanged() {
		return "", port.Refused
	}
	if err != nil || image.Config == nil {
		return "", port.Uncertain
	}
	if image.ID != c.binding.Egress.ImageID || len(image.Config.Volumes) != 0 {
		return "", port.Refused
	}
	pids, init := int64(4096), false
	config := &container.Config{Image: c.binding.Egress.ImageID, User: r.UID + ":" + r.GID, Entrypoint: []string{}, Cmd: append([]string{"/itos-container", "__egress-proxy", "--"}, c.binding.Egress.List()...), Env: slices.Clone(image.Config.Env), WorkingDir: "/", Healthcheck: &container.HealthConfig{Test: []string{"NONE"}}, Labels: egressLabels(r, "egress-proxy")}
	host := &container.HostConfig{NetworkMode: container.NetworkMode(r.Egress.Network.Name), ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"}, Resources: container.Resources{PidsLimit: &pids}, RestartPolicy: container.RestartPolicy{Name: "no"}, IpcMode: "private", CgroupnsMode: "private", Init: &init, Sysctls: map[string]string{"net.ipv4.ip_forward": "0"}, LogConfig: container.LogConfig{Type: "json-file", Config: map[string]string{"max-size": "64m", "max-file": "1", "mode": "blocking"}}, Mounts: []mount.Mount{{Type: mount.TypeBind, Source: c.executable, Target: "/itos-container", ReadOnly: true}}}
	networking := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{r.Egress.Internal.Name: {NetworkID: r.Egress.Internal.ID}, r.Egress.Network.Name: {NetworkID: r.Egress.Network.ID}}}
	reply, err := c.api.ContainerCreate(call, client.ContainerCreateOptions{Name: r.Egress.Proxy.Name, Config: config, HostConfig: host, NetworkingConfig: networking})
	if err != nil || !c.pin.Unchanged() || !port.HexID(reply.ID) {
		return "", port.Uncertain
	}
	c.proxyConfig, c.proxyRecord = config, r
	return reply.ID, ""
}
func (c *runConnection) VerifyProxy(ctx context.Context, id string) port.Outcome {
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	reply, err := c.api.ContainerInspect(call, id, client.ContainerInspectOptions{})
	if !c.pin.Unchanged() {
		return port.Refused
	}
	r := reply.Container
	if err != nil || r.Config == nil || r.HostConfig == nil || r.State == nil || r.NetworkSettings == nil {
		return port.IsolationInspection
	}
	p := c.proxyRecord
	expected := c.proxyConfig
	if r.ID != id || r.Image != c.binding.Egress.ImageID || strings.TrimPrefix(r.Name, "/") != p.Egress.Proxy.Name {
		return port.Refused
	}
	for key, value := range egressLabels(p, "egress-proxy") {
		if r.Config.Labels[key] != value {
			return port.Refused
		}
	}
	if expected == nil || r.Config.Image != expected.Image || !slices.Equal(r.Config.Cmd, expected.Cmd) || len(r.Config.Entrypoint) != 0 || r.Config.User != expected.User || r.Config.WorkingDir != "/" || !sameEnvironment(r.Config.Env, expected.Env) || r.Config.OpenStdin || r.Config.StdinOnce || r.Config.Tty || r.Config.AttachStdin || r.Config.AttachStdout || r.Config.AttachStderr || len(r.Config.ExposedPorts) != 0 || len(r.Config.Volumes) != 0 || r.Config.Healthcheck == nil || !slices.Equal(r.Config.Healthcheck.Test, []string{"NONE"}) {
		return port.IsolationUnverified
	}
	if r.State.Status != "created" || r.State.Running || r.State.Pid != 0 || (r.State.StartedAt != "" && r.State.StartedAt != "0001-01-01T00:00:00Z") || !safeProxyHost(r.HostConfig, p.Egress.Network.Name, c.executable) || !proxyMount(r.Mounts, c.executable) {
		return port.IsolationUnverified
	}
	if !egressEndpoints(r.NetworkSettings, map[string]string{p.Egress.Internal.Name: p.Egress.Internal.ID, p.Egress.Network.Name: p.Egress.Network.ID}) {
		return port.IsolationUnverified
	}
	return ""
}
func safeProxyHost(h *container.HostConfig, netname, executable string) bool {
	if h.NetworkMode != container.NetworkMode(netname) || !h.ReadonlyRootfs || h.Privileged || h.AutoRemove || h.PublishAllPorts || len(h.CapAdd) != 0 || !slices.Equal(h.CapDrop, []string{"ALL"}) || len(h.GroupAdd) != 0 || h.PidsLimit == nil || *h.PidsLimit != 4096 || h.RestartPolicy.Name != "no" || h.PidMode != "" || h.IpcMode != "private" || h.UTSMode != "" || h.UsernsMode != "" || h.CgroupnsMode != "private" || len(h.Devices) != 0 || len(h.DeviceRequests) != 0 || len(h.DeviceCgroupRules) != 0 || len(h.PortBindings) != 0 || len(h.Binds) != 0 || len(h.VolumesFrom) != 0 || len(h.Links) != 0 || len(h.ExtraHosts) != 0 || len(h.DNS) != 0 || len(h.DNSSearch) != 0 || len(h.DNSOptions) != 0 || len(h.Tmpfs) != 0 || h.Init == nil || *h.Init {
		return false
	}
	if len(h.SecurityOpt) != 1 || (h.SecurityOpt[0] != "no-new-privileges" && h.SecurityOpt[0] != "no-new-privileges=true") || len(h.Sysctls) != 1 || h.Sysctls["net.ipv4.ip_forward"] != "0" || h.LogConfig.Type != "json-file" || h.LogConfig.Config["max-size"] != "64m" || h.LogConfig.Config["max-file"] != "1" || h.LogConfig.Config["mode"] != "blocking" || len(h.Mounts) != 1 {
		return false
	}
	return h.Mounts[0] == (mount.Mount{Type: mount.TypeBind, Source: executable, Target: "/itos-container", ReadOnly: true})
}
func proxyMount(m []container.MountPoint, path string) bool {
	return len(m) == 1 && m[0].Type == mount.TypeBind && m[0].Source == path && m[0].Destination == "/itos-container" && !m[0].RW && (m[0].Propagation == "" || m[0].Propagation == mount.PropagationRPrivate)
}
func egressEndpoints(n *container.NetworkSettings, want map[string]string) bool {
	if n == nil || len(n.Networks) != len(want) {
		return false
	}
	for name, id := range want {
		ep := n.Networks[name]
		if ep == nil || ep.NetworkID != id || ep.IPAMConfig != nil || len(ep.Links) != 0 || len(ep.Aliases) != 0 || len(ep.DriverOpts) != 0 || ep.GwPriority != 0 {
			return false
		}
	}
	for _, bindings := range n.Ports {
		if len(bindings) != 0 {
			return false
		}
	}
	return true
}
