package docker

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"strings"
)

func (c *runConnection) workerNetwork(ctx context.Context, r port.Record) (*network.NetworkingConfig, string, port.Outcome) {
	if r.Schema != 3 {
		return nil, "", ""
	}
	if !r.ValidEgress(c.binding) || r.Egress.Proxy.Stage != "owned" {
		return nil, "", port.Refused
	}
	if !c.pin.Unchanged() {
		return nil, "", port.Refused
	}
	reply, err := c.api.ContainerInspect(ctx, r.Egress.Proxy.ID, client.ContainerInspectOptions{})
	if !c.pin.Unchanged() {
		return nil, "", port.Refused
	}
	p := reply.Container
	if err != nil || p.Config == nil || p.State == nil || p.NetworkSettings == nil {
		return nil, "", port.IsolationUnverified
	}
	o := port.Observation{ID: p.ID, Name: strings.TrimPrefix(p.Name, "/"), ImageID: p.Image, Token: p.Config.Labels["io.itos-container.intent"], Project: p.Config.Labels["io.itos-container.project"], Fingerprint: p.Config.Labels["io.itos-container.fingerprint"], Session: p.Config.Labels["io.itos-container.session"], Kind: p.Config.Labels["io.itos-container.kind"]}
	ep := p.NetworkSettings.Networks[r.Egress.Internal.Name]
	if !r.OwnsProxy(o, c.binding) || !p.State.Running || p.State.Status != "running" || ep == nil || ep.NetworkID != r.Egress.Internal.ID || !ep.IPAddress.Is4() {
		return nil, "", port.IsolationUnverified
	}
	return &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{r.Egress.Internal.Name: {NetworkID: r.Egress.Internal.ID}}}, ep.IPAddress.String(), ""
}
func safeWorkerHost(h *container.HostConfig, r port.Record) bool {
	if r.Schema != 3 {
		return safeHost(h, r.Anchor.Name)
	}
	if h.NetworkMode != container.NetworkMode(r.Egress.Internal.Name) || len(h.DNS) != 0 || len(h.DNSSearch) != 0 || len(h.DNSOptions) != 0 {
		return false
	}
	offline := *h
	offline.NetworkMode = "none"
	return safeHost(&offline, r.Anchor.Name)
}
