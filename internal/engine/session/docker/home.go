package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"strconv"

	"github.com/containerd/errdefs"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/client"
)

func (c *runConnection) AllocateHome(ctx context.Context, h port.HomeReference) port.Outcome {
	if !h.Valid() || h.Created != "" || !c.pin.Unchanged() {
		return port.HomeRefused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, err := c.api.VolumeCreate(call, client.VolumeCreateOptions{Name: h.Name, Driver: "local", Labels: map[string]string{
		"io.itos-container.kind": "harness-home", "io.itos-container.scope": h.Scope, "io.itos-container.token": h.ID}})
	if err != nil || !c.pin.Unchanged() {
		return port.HomeUnavailable
	}
	return ""
}
func (c *runConnection) InspectHome(ctx context.Context, name string) (port.HomeObservation, port.Inspection) {
	if !c.pin.Unchanged() {
		return port.HomeObservation{}, port.Changed
	}
	call, cancel := c.call(ctx)
	defer cancel()
	v, err := c.api.VolumeInspect(call, name, client.VolumeInspectOptions{})
	if !c.pin.Unchanged() {
		return port.HomeObservation{}, port.Changed
	}
	if errdefs.IsNotFound(err) {
		return port.HomeObservation{}, port.NotFound
	}
	if err != nil {
		return port.HomeObservation{}, port.Unavailable
	}
	o := v.Volume
	return port.HomeObservation{Name: o.Name, Created: o.CreatedAt, Driver: o.Driver, Scope: o.Scope,
		Token: o.Labels["io.itos-container.token"], Key: o.Labels["io.itos-container.scope"], EmptyOptions: len(o.Options) == 0,
		ExactLabels: len(o.Labels) == 3 && o.Labels["io.itos-container.kind"] == "harness-home"}, port.Observed
}
func (c *runConnection) InitializeHome(ctx context.Context, id, uid, gid string) port.Outcome {
	u, ue := strconv.ParseUint(uid, 10, 32)
	g, ge := strconv.ParseUint(gid, 10, 32)
	if ue != nil || ge != nil || u == 0 || !port.HexID(id) || !c.pin.Unchanged() {
		return port.HomeRefused
	}
	var data bytes.Buffer
	w := tar.NewWriter(&data)
	// The fixed directory header, bounded numeric IDs and bytes.Buffer have
	// neither an invalid tar value nor an I/O failure path.
	_ = w.WriteHeader(&tar.Header{Name: ".", Typeflag: tar.TypeDir, Mode: 0700, Uid: int(u), Gid: int(g)})
	_ = w.Close()
	call, cancel := c.call(ctx)
	defer cancel()
	_, err := c.api.CopyToContainer(call, id, client.CopyToContainerOptions{DestinationPath: "/home/itos", Content: &data, CopyUIDGID: true})
	if err != nil || !c.pin.Unchanged() {
		return port.HomeUnavailable
	}
	return ""
}
func (c *runConnection) VerifyHomeRoot(ctx context.Context, id, uid, gid string) port.Outcome {
	if !c.pin.Unchanged() {
		return port.HomeRefused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	r, err := c.api.CopyFromContainer(call, id, client.CopyFromContainerOptions{SourcePath: "/home/itos"})
	if err != nil {
		return port.HomeUnavailable
	}
	defer func() { _ = r.Content.Close() }()
	if !r.Stat.Mode.IsDir() || r.Stat.Mode.Perm() != 0700 || r.Stat.Mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || r.Stat.LinkTarget != "" {
		return port.HomeRefused
	}
	header, err := tar.NewReader(io.LimitReader(r.Content, 65536)).Next()
	if err != nil {
		return port.HomeUnavailable
	}
	if header.Typeflag != tar.TypeDir || header.Name != "itos/" && header.Name != "itos" || header.Mode != 0700 || strconv.Itoa(header.Uid) != uid || strconv.Itoa(header.Gid) != gid || !c.pin.Unchanged() {
		return port.HomeRefused
	}
	return ""
}
func (c *runConnection) RemoveHome(ctx context.Context, name string) port.Outcome {
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, err := c.api.VolumeRemove(call, name, client.VolumeRemoveOptions{Force: false})
	if !c.pin.Unchanged() {
		return port.Refused
	}
	if err != nil {
		return port.Incomplete
	}
	return ""
}
