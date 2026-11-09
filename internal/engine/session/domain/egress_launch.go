package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func prepareEgress(ctx context.Context, i port.Intent, c port.RunConnection) port.Outcome {
	if i.Binding().Egress == (port.EgressGrant{}) {
		return ""
	}
	r := i.Record()
	r.Schema = 3
	r.Egress = port.EgressRecord{Internal: port.EgressResource{Name: "itos-container-internal-" + r.NameToken()}, Network: port.EgressResource{Name: "itos-container-egress-" + r.NameToken()}, Proxy: port.EgressResource{Name: "itos-container-proxy-" + r.NameToken()}}
	for _, kind := range []port.NetworkKind{port.InternalNetwork, port.EgressNetwork} {
		resource := &r.Egress.Internal
		if kind == port.EgressNetwork {
			resource = &r.Egress.Network
		}
		resource.Stage = "intent"
		if f := i.Save(r); f != "" {
			return f
		}
		if f := c.Check(ctx); f != "" {
			return f
		}
		var id string
		f := startupOperation(ctx, "network creation", func(call context.Context) port.Outcome {
			var f port.Outcome
			id, f = c.CreateNetwork(call, r, kind)
			return f
		})
		if f != "" {
			return f
		}
		if !port.HexID(id) {
			return port.Uncertain
		}
		var o port.NetworkObservation
		f = startupOperation(ctx, "network inspection", func(call context.Context) port.Outcome {
			var status port.Inspection
			o, status = c.InspectNetwork(call, id)
			if status == port.Changed {
				return port.Refused
			}
			if status != port.Observed {
				return port.IsolationInspection
			}
			return ""
		})
		if f != "" {
			return f
		}
		if o.ID != id || !r.OwnsNetwork(o, kind) {
			return port.Refused
		}
		resource.ID, resource.Stage = id, "owned"
		if i.Save(r) != "" {
			return port.OwnershipWrite
		}
		if !r.ExactNetwork(o, kind) || o.Endpoints != 0 {
			return port.IsolationUnverified
		}
	}
	r.Egress.Proxy.Stage = "intent"
	if f := i.Save(r); f != "" {
		return f
	}
	if f := c.Check(ctx); f != "" {
		return f
	}
	var id string
	f := startupOperation(ctx, "proxy creation", func(call context.Context) port.Outcome { var f port.Outcome; id, f = c.CreateProxy(call, r); return f })
	if f != "" {
		return f
	}
	if !port.HexID(id) {
		return port.Uncertain
	}
	f = startupOperation(ctx, "proxy inspection", func(call context.Context) port.Outcome {
		o, status := c.Inspect(call, id)
		if status == port.Changed {
			return port.Refused
		}
		if status != port.Observed {
			return port.IsolationInspection
		}
		if o.ID != id || !r.OwnsProxy(o, i.Binding()) {
			return port.Refused
		}
		return ""
	})
	if f != "" {
		return f
	}
	r.Egress.Proxy.ID, r.Egress.Proxy.Stage = id, "owned"
	if i.Save(r) != "" {
		return port.OwnershipWrite
	}
	for _, op := range []struct {
		name string
		call func(context.Context, string) port.Outcome
	}{{"proxy verification", c.VerifyProxy}, {"proxy start", c.Start}, {"proxy readiness", c.ProxyReady}} {
		if f := startupOperation(ctx, op.name, func(call context.Context) port.Outcome { return op.call(call, id) }); f != "" {
			return f
		}
	}
	return verifyLiveProxy(ctx, r, i.Binding(), c)
}
func verifyLiveProxy(ctx context.Context, r port.Record, b port.Binding, c port.RunConnection) port.Outcome {
	return startupOperation(ctx, "proxy inspection", func(call context.Context) port.Outcome {
		o, status := c.Inspect(call, r.Egress.Proxy.ID)
		if status != port.Observed || !r.OwnsProxy(o, b) || !o.StatePresent || !o.Running || o.Status != "running" {
			return port.ProxyFailed
		}
		return ""
	})
}
func verifyEgressStart(ctx context.Context, i port.Intent, c port.RunConnection) port.Outcome {
	r := i.Record()
	if r.Schema != 3 {
		return ""
	}
	if f := verifyLiveProxy(ctx, r, i.Binding(), c); f != "" {
		return f
	}
	for _, kind := range []port.NetworkKind{port.InternalNetwork, port.EgressNetwork} {
		id := r.Egress.Internal.ID
		if kind == port.EgressNetwork {
			id = r.Egress.Network.ID
		}
		f := startupOperation(ctx, "network inspection", func(call context.Context) port.Outcome {
			o, status := c.InspectNetwork(call, id)
			if status == port.Changed {
				return port.Refused
			}
			if status != port.Observed {
				return port.IsolationInspection
			}
			if !r.OwnsNetwork(o, kind) {
				return port.Refused
			}
			if !r.ExactNetwork(o, kind) || o.Endpoints != 1 || o.ContainerID != r.Egress.Proxy.ID {
				return port.IsolationUnverified
			}
			return ""
		})
		if f != "" {
			return f
		}
	}
	return ""
}
