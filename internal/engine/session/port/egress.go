package port

import (
	"context"
	"strings"
)

const (
	ProxyFailed   Outcome = "proxy-failed"
	ProxyExited   Outcome = "proxy-exited"
	RefusalLogGap Outcome = "refusal-log-gap"
)

type EgressGrant struct{ Names, ImageID string }

func (g EgressGrant) List() []string {
	if g.Names == "" {
		return nil
	}
	return strings.Split(g.Names, "\n")
}
func (g EgressGrant) Valid() bool {
	if g == (EgressGrant{}) {
		return true
	}
	if !strings.HasPrefix(g.ImageID, "sha256:") || !HexID(strings.TrimPrefix(g.ImageID, "sha256:")) {
		return false
	}
	for _, name := range strings.Split(g.Names, "\n") {
		if name == "" {
			return false
		}
	}
	return true
}

type EgressResource struct {
	Name  string `json:"name"`
	ID    string `json:"id"`
	Stage string `json:"stage"`
}
type EgressRecord struct {
	Internal EgressResource `json:"internal"`
	Network  EgressResource `json:"network"`
	Proxy    EgressResource `json:"proxy"`
}
type NetworkKind string

const (
	InternalNetwork NetworkKind = "internal-network"
	EgressNetwork   NetworkKind = "egress-network"
)

type NetworkObservation struct {
	ID, Name, Driver, Scope                     string
	Internal, IPv6, Isolated, Gateway           bool
	ConfigOnly, Ingress, Attachable, FromConfig bool
	Token, Project, Fingerprint, Session, Kind  string
	ExactLabels                                 bool
	Endpoints                                   int
	ContainerID                                 string
}
type EgressConnection interface {
	CreateNetwork(context.Context, Record, NetworkKind) (string, Outcome)
	InspectNetwork(context.Context, string) (NetworkObservation, Inspection)
	RemoveNetwork(context.Context, string)
	CreateProxy(context.Context, Record) (string, Outcome)
	VerifyProxy(context.Context, string) Outcome
	ProxyReady(context.Context, string) Outcome
	CopyProxyLog(context.Context, string, Writer) Outcome
}

// ValidEgress binds persisted progress to the frozen grant without enabling it.
func (r Record) ValidEgress(b Binding) bool {
	if r.Schema != 3 {
		return r.Egress == (EgressRecord{}) && (b.Egress == (EgressGrant{}) || r.Stage == "reserved" || r.Stage == "cancelled")
	}
	return b.Egress != (EgressGrant{}) && b.Egress.Valid() && r.ValidEgressShape()
}
func (r Record) EgressGone() bool {
	for _, v := range []EgressResource{r.Egress.Internal, r.Egress.Network, r.Egress.Proxy} {
		if v.Stage != "" && v.Stage != "absent" {
			return false
		}
	}
	return true
}

// ValidEgressShape checks the independently decoded resource authority.
func (r Record) ValidEgressShape() bool {
	if r.Schema != 3 {
		return r.Egress == (EgressRecord{})
	}
	values := []EgressResource{r.Egress.Internal, r.Egress.Network, r.Egress.Proxy}
	for i, v := range values {
		prefix := []string{"internal", "egress", "proxy"}[i]
		if v.Name != "itos-container-"+prefix+"-"+r.NameToken() {
			return false
		}
		switch v.Stage {
		case "", "intent":
			if v.ID != "" {
				return false
			}
		case "owned", "absent":
			if !HexID(v.ID) {
				return false
			}
		default:
			return false
		}
		if v.Stage != "" {
			for _, earlier := range values[:i] {
				if earlier.Stage != "owned" && earlier.Stage != "absent" {
					return false
				}
			}
		}
		if v.Stage == "absent" {
			for _, later := range values[i+1:] {
				if later.Stage != "" && later.Stage != "absent" {
					return false
				}
			}
		}
		if (r.Stage == "intent" || r.Stage == "owned") && v.Stage != "owned" {
			return false
		}
	}
	return r.Anchor.Stage == "owned" || r.Anchor.Stage == "absent" && r.EgressGone()
}
func (r Record) OwnsNetwork(o NetworkObservation, k NetworkKind) bool {
	v := r.Egress.Network
	if k == InternalNetwork {
		v = r.Egress.Internal
	} else if k != EgressNetwork {
		return false
	}
	return HexID(o.ID) && (v.ID == "" || v.ID == o.ID) && o.Name == v.Name && o.Driver == "bridge" && o.Scope == "local" && o.ExactLabels &&
		o.Token == r.NameToken() && o.Project == r.Project && o.Fingerprint == r.Fingerprint && o.Session == r.Anchor.Session && o.Kind == string(k)
}
func (r Record) ExactNetwork(o NetworkObservation, k NetworkKind) bool {
	return r.OwnsNetwork(o, k) && !o.IPv6 && !o.ConfigOnly && !o.Ingress && !o.Attachable && !o.FromConfig &&
		((k == InternalNetwork && o.Internal && o.Isolated && !o.Gateway) || (k == EgressNetwork && !o.Internal))
}
func (r Record) OwnsProxy(o Observation, b Binding) bool {
	v := r.Egress.Proxy
	return HexID(o.ID) && (v.ID == "" || v.ID == o.ID) && o.Name == v.Name && o.ImageID == b.Egress.ImageID &&
		o.Token == r.NameToken() && o.Project == r.Project && o.Fingerprint == r.Fingerprint && o.Session == r.Anchor.Session && o.Kind == "egress-proxy"
}

type RefusalGap struct{ First, Last uint64 }
type RefusalReport struct {
	Outcome Outcome
	Gaps    []RefusalGap
}
type RefusalLog interface {
	Begin(context.Context, string) (RefusalCopy, Outcome)
}
type RefusalCopy interface {
	Writer
	Commit() (RefusalReport, Outcome)
	Close()
}
