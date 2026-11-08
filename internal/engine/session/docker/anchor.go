package docker

import (
	"context"

	"github.com/containerd/errdefs"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/client"
)

func (c *runConnection) AllocateAnchor(ctx context.Context, r port.Record) port.Outcome {
	if !c.pin.Unchanged() || !r.ValidAnchor(c.binding) || r.Anchor.Stage != "intent" {
		return port.MountRefused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, err := c.api.VolumeCreate(call, client.VolumeCreateOptions{
		Name: r.Anchor.Name, Driver: "local",
		DriverOpts: map[string]string{"type": "none", "o": "bind", "device": r.Anchor.Source},
		Labels: map[string]string{"io.itos-container.intent": r.NameToken(), "io.itos-container.project": r.Project,
			"io.itos-container.fingerprint": r.Fingerprint, "io.itos-container.session": r.Anchor.Session,
			"io.itos-container.device": r.Anchor.Device, "io.itos-container.inode": r.Anchor.Inode, "io.itos-container.kind": "checkout-anchor"},
	})
	if err != nil || !c.pin.Unchanged() {
		return port.MountUncertain
	}
	return ""
}

func (c *runConnection) InspectAnchor(ctx context.Context, name string) (port.AnchorObservation, port.Inspection) {
	if !c.pin.Unchanged() {
		return port.AnchorObservation{}, port.Changed
	}
	call, cancel := c.call(ctx)
	defer cancel()
	result, err := c.api.VolumeInspect(call, name, client.VolumeInspectOptions{})
	if !c.pin.Unchanged() {
		return port.AnchorObservation{}, port.Changed
	}
	if errdefs.IsNotFound(err) {
		return port.AnchorObservation{}, port.NotFound
	}
	if err != nil {
		return port.AnchorObservation{}, port.Unavailable
	}
	v := result.Volume
	return port.AnchorObservation{Name: v.Name, Created: v.CreatedAt, Driver: v.Driver, Scope: v.Scope,
		Source: v.Options["device"], Type: v.Options["type"], Options: v.Options["o"], ExactOptions: len(v.Options) == 3,
		Token: v.Labels["io.itos-container.intent"], Project: v.Labels["io.itos-container.project"], Fingerprint: v.Labels["io.itos-container.fingerprint"],
		Session: v.Labels["io.itos-container.session"], Device: v.Labels["io.itos-container.device"], Inode: v.Labels["io.itos-container.inode"],
		ExactLabels: len(v.Labels) == 7 && v.Labels["io.itos-container.kind"] == "checkout-anchor"}, port.Observed
}

func (c *runConnection) RemoveAnchor(ctx context.Context, name string) {
	if !c.pin.Unchanged() {
		return
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, _ = c.api.VolumeRemove(call, name, client.VolumeRemoveOptions{})
}
