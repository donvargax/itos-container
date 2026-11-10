package docker

import (
	"context"
	"github.com/containerd/errdefs"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/client"
)

const isolatedOption = "com.docker.network.bridge.gateway_mode_ipv4"

func egressLabels(r port.Record, kind string) map[string]string {
	return map[string]string{"io.itos-container.intent": r.NameToken(), "io.itos-container.project": r.Project, "io.itos-container.fingerprint": r.Fingerprint, "io.itos-container.session": r.Anchor.Session, "io.itos-container.kind": kind}
}
func (c *runConnection) CreateNetwork(ctx context.Context, r port.Record, k port.NetworkKind) (string, port.Outcome) {
	if !c.pin.Unchanged() {
		return "", port.Refused
	}
	if r.Schema != 3 || !r.ValidEgress(c.binding) || r.Stage != "reserved" {
		return "", port.Refused
	}
	resource := r.Egress.Internal
	if k == port.EgressNetwork {
		resource = r.Egress.Network
		if r.Egress.Internal.Stage != "owned" {
			return "", port.Refused
		}
	} else if k != port.InternalNetwork {
		return "", port.Refused
	}
	if resource.Stage != "intent" {
		return "", port.Refused
	}
	ipv6 := false
	options := client.NetworkCreateOptions{Driver: "bridge", Internal: k == port.InternalNetwork, EnableIPv6: &ipv6, Labels: egressLabels(r, string(k))}
	if k == port.InternalNetwork {
		options.Options = map[string]string{isolatedOption: "isolated"}
	}
	call, cancel := c.call(ctx)
	defer cancel()
	reply, err := c.api.NetworkCreate(call, resource.Name, options)
	if err != nil || !c.pin.Unchanged() || !port.HexID(reply.ID) {
		return "", port.Uncertain
	}
	return reply.ID, ""
}
func (c *runConnection) InspectNetwork(ctx context.Context, id string) (port.NetworkObservation, port.Inspection) {
	if !c.pin.Unchanged() {
		return port.NetworkObservation{}, port.Changed
	}
	call, cancel := c.call(ctx)
	defer cancel()
	reply, err := c.api.NetworkInspect(call, id, client.NetworkInspectOptions{})
	if !c.pin.Unchanged() {
		return port.NetworkObservation{}, port.Changed
	}
	if errdefs.IsNotFound(err) {
		return port.NetworkObservation{}, port.NotFound
	}
	r := reply.Network
	if err != nil || r.ID == "" || r.Name == "" {
		return port.NetworkObservation{}, port.Unavailable
	}
	o := port.NetworkObservation{ID: r.ID, Name: r.Name, Driver: r.Driver, Scope: r.Scope, Internal: r.Internal, IPv6: r.EnableIPv6, Isolated: r.Options[isolatedOption] == "isolated", ConfigOnly: r.ConfigOnly, Ingress: r.Ingress, Attachable: r.Attachable, FromConfig: r.ConfigFrom.Network != "", ExactLabels: len(r.Labels) == 5, Endpoints: len(r.Containers), Token: r.Labels["io.itos-container.intent"], Project: r.Labels["io.itos-container.project"], Fingerprint: r.Labels["io.itos-container.fingerprint"], Session: r.Labels["io.itos-container.session"], Kind: r.Labels["io.itos-container.kind"]}
	for _, v := range r.IPAM.Config {
		if v.Gateway.IsValid() {
			o.Gateway = true
		}
	}
	if o.Endpoints == 1 {
		for id := range r.Containers {
			o.ContainerID = id
		}
	}
	return o, port.Observed
}
func (c *runConnection) RemoveNetwork(ctx context.Context, id string) {
	if !c.pin.Unchanged() || !port.HexID(id) {
		return
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, _ = c.api.NetworkRemove(call, id, client.NetworkRemoveOptions{})
}
