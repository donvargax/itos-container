package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"strings"
)

const ProxyStdout = "{\"schema\":1,\"seq\":1,\"time\":\"2026-11-09T12:00:00Z\",\"event\":\"ready\"}\n{\"schema\":1,\"seq\":2,\"time\":\"2026-11-09T12:00:01Z\",\"event\":\"refused\",\"reason\":\"not-listed\"}\n"
const ProxyStderr = "proxy stopped serving\n"

func EgressBinding(b port.Binding) port.Binding {
	b.Egress = port.EgressGrant{Names: "one.example\ntwo.example", ImageID: "sha256:" + strings.Repeat("e", 64)}
	return b
}

// EgressRecords enumerates each durable boundary independently of predicates.
func EgressRecords(b port.Binding) []port.Record {
	r := RunIntent(b)
	r.Stage = "reserved"
	result := []port.Record{r}
	r.Schema = 3
	r.Egress = port.EgressRecord{
		Internal: port.EgressResource{Name: "itos-container-internal-" + r.NameToken()},
		Network:  port.EgressResource{Name: "itos-container-egress-" + r.NameToken()},
		Proxy:    port.EgressResource{Name: "itos-container-proxy-" + r.NameToken()},
	}
	for index, resource := range []*port.EgressResource{&r.Egress.Internal, &r.Egress.Network, &r.Egress.Proxy} {
		resource.Stage = "intent"
		result = append(result, r)
		resource.Stage, resource.ID = "owned", strings.Repeat(string(rune('a'+index)), 64)
		result = append(result, r)
	}
	r.Stage = "intent"
	result = append(result, r)
	r.Stage, r.ContainerID = "owned", strings.Repeat("d", 64)
	result = append(result, r)
	r.Stage = "absent"
	result = append(result, r)
	for _, resource := range []*port.EgressResource{&r.Egress.Proxy, &r.Egress.Network, &r.Egress.Internal} {
		resource.Stage = "absent"
		result = append(result, r)
	}
	r.Anchor.Stage = "absent"
	return append(result, r)
}
func EgressNetwork(r port.Record, k port.NetworkKind) port.NetworkObservation {
	resource := r.Egress.Network
	if k == port.InternalNetwork {
		resource = r.Egress.Internal
	}
	return port.NetworkObservation{ID: resource.ID, Name: resource.Name, Driver: "bridge", Scope: "local",
		Internal: k == port.InternalNetwork, Isolated: k == port.InternalNetwork, Gateway: k == port.EgressNetwork,
		Token: r.NameToken(), Project: r.Project, Fingerprint: r.Fingerprint, Session: r.Anchor.Session, Kind: string(k), ExactLabels: true}
}
func EgressProxy(r port.Record, b port.Binding) port.Observation {
	return port.Observation{ID: r.Egress.Proxy.ID, Name: r.Egress.Proxy.Name, ImageID: b.Egress.ImageID,
		Token: r.NameToken(), Project: r.Project, Fingerprint: r.Fingerprint, Session: r.Anchor.Session, Kind: "egress-proxy", StatePresent: true, Status: "created"}
}
