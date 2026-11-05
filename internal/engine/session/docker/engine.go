// Package docker implements only unstarted create, independent inspect and remove.
package docker

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	readiness "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Engine struct {
	Readiness rp.Engine
	Selection string
}
type connection struct {
	api       *client.Client
	pin       rp.Session
	transport *http.Transport
	binding   port.Binding
	ctx       context.Context
}

func (e Engine) Open(ctx context.Context, b port.Binding) (port.Connection, port.Outcome) {
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if call.Err() != nil {
		return nil, port.Incomplete
	}
	deadline, _ := call.Deadline()
	pin, f := e.Readiness.Open(e.Selection, b.Checkout.Root, time.Until(deadline))
	if f != rp.None {
		if f == rp.EngineChanged || f == rp.EngineRemote || f == rp.EngineSelection || f == rp.EngineUntrusted {
			return nil, port.Refused
		}
		return nil, port.Incomplete
	}
	success := false
	defer func() {
		if !success {
			pin.Close()
		}
	}()
	if pin.Endpoint() != b.Endpoint {
		return nil, port.Refused
	}
	version, f := pin.Version()
	if f != rp.None {
		return nil, port.Incomplete
	}
	info, f := pin.Info()
	if f != rp.None {
		return nil, port.Incomplete
	}
	if readiness.ValidateEngine(version, info) != rp.None || info.ID != b.EngineID || !pin.Unchanged() {
		return nil, port.Refused
	}
	path := strings.TrimPrefix(pin.Endpoint(), "unix://")
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		if !pin.Unchanged() {
			return nil, errors.New("engine socket changed")
		}
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	api, err := client.New(client.WithHost(pin.Endpoint()), client.WithAPIVersion(version.Negotiated), client.WithHTTPClient(&http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}))
	if err != nil {
		transport.CloseIdleConnections()
		return nil, port.Incomplete
	}
	c := &connection{api: api, pin: pin, transport: transport, binding: b, ctx: ctx}
	success = true
	return c, ""
}
func (c *connection) call(ctx context.Context) (context.Context, context.CancelFunc) {
	// Both the original operation deadline and each call's limit survive caller
	// reconstruction/reconnect. No method replaces the operation budget.
	if deadline, ok := c.ctx.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		call, end := context.WithTimeout(ctx, 10*time.Second)
		return call, func() { end(); cancel() }
	}
	return context.WithTimeout(ctx, 10*time.Second)
}
func (c *connection) Check(ctx context.Context) port.Outcome {
	if !c.pin.Unchanged() {
		return port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	info, err := c.api.Info(call, client.InfoOptions{})
	if !c.pin.Unchanged() {
		return port.Refused
	}
	if err != nil {
		return port.Incomplete
	}
	if info.Info.ID != c.binding.EngineID {
		return port.Refused
	}
	return ""
}
func (c *connection) Create(ctx context.Context, r port.Record) (string, port.Outcome) {
	if !c.pin.Unchanged() {
		return "", port.Refused
	}
	call, cancel := c.call(ctx)
	defer cancel()
	pids := int64(4096)
	result, err := c.api.ContainerCreate(call, client.ContainerCreateOptions{Name: r.Name,
		Config:     &container.Config{Image: r.ImageReference, User: r.UID + ":" + r.GID, Cmd: []string{"/bin/true"}, Entrypoint: []string{}, Labels: map[string]string{"io.itos-container.intent": r.NameToken(), "io.itos-container.project": r.Project, "io.itos-container.fingerprint": r.Fingerprint}},
		HostConfig: &container.HostConfig{NetworkMode: "none", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"}, Resources: container.Resources{PidsLimit: &pids}, RestartPolicy: container.RestartPolicy{Name: "no"}},
	})
	if err != nil || !c.pin.Unchanged() || !port.HexID(result.ID) {
		return "", port.Uncertain
	}
	return result.ID, ""
}
func (c *connection) Inspect(ctx context.Context, id string) (port.Observation, port.Inspection) {
	if !c.pin.Unchanged() {
		return port.Observation{}, port.Changed
	}
	call, cancel := c.call(ctx)
	defer cancel()
	result, err := c.api.ContainerInspect(call, id, client.ContainerInspectOptions{})
	if !c.pin.Unchanged() {
		return port.Observation{}, port.Changed
	}
	if errdefs.IsNotFound(err) {
		return port.Observation{}, port.NotFound
	}
	if err != nil {
		return port.Observation{}, port.Unavailable
	}
	r := result.Container
	if r.Config == nil || r.ID == "" || r.Name == "" || r.Image == "" {
		return port.Observation{}, port.Unavailable
	}
	observed := port.Observation{ID: r.ID, Name: strings.TrimPrefix(r.Name, "/"), ImageID: r.Image, Token: r.Config.Labels["io.itos-container.intent"], Project: r.Config.Labels["io.itos-container.project"], Fingerprint: r.Config.Labels["io.itos-container.fingerprint"], StatePresent: r.State != nil}
	if r.State != nil {
		observed.Status = string(r.State.Status)
		observed.Running = r.State.Running
		observed.PID = r.State.Pid
		observed.Started = r.State.StartedAt != "" && r.State.StartedAt != "0001-01-01T00:00:00Z"
	}
	return observed, port.Observed
}
func (c *connection) Remove(ctx context.Context, id string) {
	if !c.pin.Unchanged() {
		return
	}
	call, cancel := c.call(ctx)
	defer cancel()
	_, _ = c.api.ContainerRemove(call, id, client.ContainerRemoveOptions{})
}
func (c *connection) Close() { _ = c.api.Close(); c.transport.CloseIdleConnections(); c.pin.Close() }
