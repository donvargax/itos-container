package porttest

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"strings"
)

func EgressConnectionContract(t Reporter, open func() (port.RunEngine, port.Binding)) {
	t.Helper()
	ctx := context.Background()
	e, b := open()
	c, f := e.OpenRun(ctx, b)
	if f != "" {
		t.Errorf("open: %s", f)
		return
	}
	defer c.Close()
	r := EgressRecords(b)[1]
	if o, s := c.InspectNetwork(ctx, r.Egress.Internal.Name); s != port.NotFound || o != (port.NetworkObservation{}) {
		t.Errorf("missing network = %#v %v", o, s)
	}
	for _, kind := range []port.NetworkKind{port.InternalNetwork, port.EgressNetwork} {
		resource := &r.Egress.Internal
		if kind == port.EgressNetwork {
			resource = &r.Egress.Network
		}
		resource.Stage = "intent"
		id, f := c.CreateNetwork(ctx, r, kind)
		if f != "" || !port.HexID(id) {
			t.Errorf("network creation: %q %s", id, f)
			return
		}
		resource.ID, resource.Stage = id, "owned"
		o, s := c.InspectNetwork(ctx, id)
		if s != port.Observed || !r.OwnsNetwork(o, kind) || !r.ExactNetwork(o, kind) || o.Endpoints != 0 {
			t.Errorf("network inspection: %#v %v", o, s)
		}
		if byName, s := c.InspectNetwork(ctx, resource.Name); s != port.Observed || byName != o {
			t.Errorf("network name differs")
		}
	}
	r.Egress.Proxy.Stage = "intent"
	id, f := c.CreateProxy(ctx, r)
	if f != "" || !port.HexID(id) {
		t.Errorf("proxy creation: %q %s", id, f)
		return
	}
	r.Egress.Proxy.ID, r.Egress.Proxy.Stage = id, "owned"
	if o, s := c.Inspect(ctx, id); s != port.Observed || !r.OwnsProxy(o, b) || o.Started || o.Running {
		t.Errorf("proxy observation: %#v %v", o, s)
	}
	if f = c.VerifyProxy(ctx, id); f != "" {
		t.Errorf("proxy verification: %s", f)
	}
	if f = c.Start(ctx, id); f != "" {
		t.Errorf("proxy start: %s", f)
	}
	if f = c.ProxyReady(ctx, id); f != "" {
		t.Errorf("ready after stderr: %s", f)
	}
	if f = c.Signal(ctx, id, "SIGKILL"); f != "" {
		t.Errorf("proxy signal: %s", f)
	}
	if _, f = c.Wait(ctx, id); f != "" {
		t.Errorf("proxy wait: %s", f)
	}
	if o, s := c.Inspect(ctx, id); s != port.Observed || o.Running || o.Status != "exited" {
		t.Errorf("proxy did not exit: %#v %v", o, s)
	}
	var copy strings.Builder
	if f = c.CopyProxyLog(ctx, id, &copy); f != "" || copy.String() != ProxyStdout {
		t.Errorf("stdout copy: %q %s", copy.String(), f)
	}
	c.RemoveRun(ctx, id)
	if _, s := c.Inspect(ctx, id); s != port.NotFound {
		t.Errorf("proxy remains")
	}
	for _, id := range []string{r.Egress.Network.ID, r.Egress.Internal.ID} {
		c.RemoveNetwork(ctx, id)
		if _, s := c.InspectNetwork(ctx, id); s != port.NotFound {
			t.Errorf("network remains")
		}
	}
}
