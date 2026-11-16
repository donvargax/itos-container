package porttest

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"strings"
)

func (c *runConnection) CreateNetwork(ctx context.Context, r port.Record, k port.NetworkKind) (string, port.Outcome) {
	event := string(k) + " create"
	c.run.event(event)
	if c.engine.Replaced {
		return "", port.Refused
	}
	if ctx.Err() != nil || c.run.EgressFault == event {
		return "", port.Uncertain
	}
	o := EgressNetwork(r, k)
	o.ID = strings.Repeat("a", 64)
	if k == port.EgressNetwork {
		o.ID = strings.Repeat("b", 64)
	}
	if c.run.EgressFault == "internal-network exact" && k == port.InternalNetwork {
		o.Gateway = true
	}
	if c.run.EgressFault == string(k)+" occupied at create" {
		o.Endpoints, o.ContainerID = 1, strings.Repeat("f", 64)
	}
	c.engine.resourcesMu.Lock()
	if c.run.Networks == nil {
		c.run.Networks = map[string]port.NetworkObservation{}
	}
	c.run.Networks[o.ID] = o
	c.engine.resourcesMu.Unlock()
	c.egressCreateEvent(r, "NETWORK CREATE", o.Name)
	if c.run.EgressFault == string(k)+" short ID" {
		return "short", ""
	}
	return o.ID, ""
}
func (c *runConnection) egressCreateEvent(r port.Record, method, name string) {
	c.engine.Events = append(c.engine.Events, EngineEvent{Method: method, Target: name, Record: r, Intent: string(c.engine.Store.Records[port.Reference(r.Project, r.NameToken())])})
}
func (c *runConnection) InspectNetwork(ctx context.Context, target string) (port.NetworkObservation, port.Inspection) {
	c.run.event("network inspect")
	if c.run.EgressFault == "network unavailable" {
		return port.NetworkObservation{}, port.Unavailable
	}
	if c.run.EgressFault == "network changed" {
		return port.NetworkObservation{}, port.Changed
	}
	if c.engine.Replaced {
		return port.NetworkObservation{}, port.Changed
	}
	if ctx.Err() != nil {
		return port.NetworkObservation{}, port.Unavailable
	}
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	for id, o := range c.run.Networks {
		if target == id || target == o.Name {
			if c.run.EgressFault == "network foreign" {
				o.Kind = "foreign"
			}
			if c.run.EgressFault == "foreign endpoint" && o.Endpoints > 0 {
				o.Endpoints = 2
				o.ContainerID = ""
			}
			if c.run.EgressFault == o.Kind+" foreign single endpoint" && o.Endpoints > 0 {
				o.Endpoints, o.ContainerID = 1, strings.Repeat("f", 64)
			}
			return o, port.Observed
		}
	}
	return port.NetworkObservation{}, port.NotFound
}
func (c *runConnection) RemoveNetwork(ctx context.Context, id string) {
	c.engine.resourcesMu.Lock()
	o := c.run.Networks[id]
	c.engine.resourcesMu.Unlock()
	c.run.event(o.Kind + " remove")
	if ctx.Err() != nil || c.engine.Replaced || c.run.EgressFault == o.Kind+" remove" {
		return
	}
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	if o.Endpoints == 0 {
		delete(c.run.Networks, id)
	}
}
func (c *runConnection) CreateProxy(ctx context.Context, r port.Record) (string, port.Outcome) {
	c.run.event("proxy create")
	if c.engine.Replaced {
		return "", port.Refused
	}
	if ctx.Err() != nil || c.run.EgressFault == "proxy create" {
		return "", port.Uncertain
	}
	o := EgressProxy(r, c.binding)
	o.ID = strings.Repeat("e", 64)
	c.engine.resourcesMu.Lock()
	c.engine.Resources[o.ID] = o
	c.run.ProxyID = o.ID
	c.engine.resourcesMu.Unlock()
	c.egressCreateEvent(r, "PROXY CREATE", r.Egress.Proxy.Name)
	if c.run.EgressFault == "proxy short ID" {
		return "short", ""
	}
	return o.ID, ""
}
func (c *runConnection) VerifyProxy(ctx context.Context, id string) port.Outcome {
	c.run.event("proxy verify")
	if c.engine.Replaced {
		return port.Refused
	}
	if ctx.Err() != nil {
		return port.IsolationInspection
	}
	if c.run.EgressFault == "proxy verify" {
		return port.IsolationUnverified
	}
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	o, ok := c.engine.Resources[id]
	if !ok || o.Status != "created" || o.Running || o.Started {
		return port.IsolationUnverified
	}
	return ""
}
func (c *runConnection) ProxyReady(ctx context.Context, id string) port.Outcome {
	c.run.event("proxy ready")
	if c.engine.Replaced {
		return port.Refused
	}
	if ctx.Err() != nil {
		return port.ExecutionFailed
	}
	if c.run.EgressFault == "proxy silent" {
		<-ctx.Done()
		return port.ProxyFailed
	}
	if c.run.EgressFault == "proxy ready" {
		return port.ProxyFailed
	}
	if c.run.EgressFault == "proxy stopped" {
		c.engine.resourcesMu.Lock()
		o := c.engine.Resources[id]
		o.Running, o.Status, o.PID = false, "exited", 0
		c.engine.Resources[id] = o
		c.engine.resourcesMu.Unlock()
	}
	return ""
}
func (c *runConnection) CopyProxyLog(ctx context.Context, _ string, w port.Writer) port.Outcome {
	c.run.event("proxy copy")
	if c.engine.Replaced {
		return port.Refused
	}
	if c.run.EgressFault == "proxy copy timeout" {
		<-ctx.Done()
		return port.ExecutionFailed
	}
	if ctx.Err() != nil || c.run.EgressFault == "proxy copy" {
		return port.ExecutionFailed
	}
	data := c.run.ProxyOutput
	if data == "" {
		data = ProxyStdout
	}
	if n, err := w.Write([]byte(data)); err != nil || n != len(data) {
		return port.ExecutionFailed
	}
	return ""
}
func (c *runConnection) isProxy(id string) bool {
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	return id == c.run.ProxyID || c.engine.Resources[id].Kind == "egress-proxy"
}
func (c *runConnection) startProxy(ctx context.Context, id string) port.Outcome {
	c.run.event("proxy start")
	if ctx.Err() != nil || c.run.EgressFault == "proxy start" {
		return port.ExecutionFailed
	}
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	o := c.engine.Resources[id]
	o.Running, o.Started, o.Status, o.PID = true, true, "running", 124
	c.engine.Resources[id] = o
	for k, n := range c.run.Networks {
		n.Endpoints, n.ContainerID = 1, id
		c.run.Networks[k] = n
	}
	return ""
}
func (c *runConnection) waitProxy(ctx context.Context, id string) (int, port.Outcome) {
	c.run.event("proxy wait")
	if c.run.EgressFault == "proxy wait" {
		return 0, port.ExecutionFailed
	}
	c.engine.resourcesMu.Lock()
	running := c.engine.Resources[id].Running
	c.engine.resourcesMu.Unlock()
	if running {
		select {
		case <-c.run.ProxyExit:
		case <-ctx.Done():
			return 0, port.ExecutionFailed
		}
	}
	c.engine.resourcesMu.Lock()
	o := c.engine.Resources[id]
	o.Running, o.Status, o.PID = false, "exited", 0
	c.engine.Resources[id] = o
	c.engine.resourcesMu.Unlock()
	if c.run.AfterProxyWait != nil {
		c.run.AfterProxyWait()
	}
	return 137, ""
}
func (c *runConnection) signalProxy(ctx context.Context, id, signal string) port.Outcome {
	c.run.event("proxy " + signal)
	if c.run.EgressFault == "proxy signal" {
		return port.ExecutionFailed
	}
	if c.run.EgressFault == "proxy ignores kill" {
		return ""
	}
	if ctx.Err() != nil {
		return port.ExecutionFailed
	}
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	o := c.engine.Resources[id]
	o.Running, o.Status, o.PID = false, "exited", 0
	c.engine.Resources[id] = o
	return ""
}
func (c *runConnection) removeProxy(ctx context.Context, id string) {
	c.run.event("proxy remove")
	if ctx.Err() != nil || c.engine.Replaced {
		return
	}
	c.engine.resourcesMu.Lock()
	defer c.engine.resourcesMu.Unlock()
	delete(c.engine.Resources, id)
	for k, n := range c.run.Networks {
		if n.ContainerID == id {
			n.Endpoints, n.ContainerID = 0, ""
			c.run.Networks[k] = n
		}
	}
}
