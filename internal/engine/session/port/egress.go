package port

import "context"

const (
	ProxyFailed   Outcome = "proxy-failed"
	ProxyExited   Outcome = "proxy-exited"
	RefusalLogGap Outcome = "refusal-log-gap"
)

type EgressGrant struct{ Names, ImageID string }

func (EgressGrant) List() []string { return nil }
func (EgressGrant) Valid() bool    { return false }

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

func (Record) ValidEgress(Binding) bool                          { return false }
func (Record) OwnsNetwork(NetworkObservation, NetworkKind) bool  { return false }
func (Record) ExactNetwork(NetworkObservation, NetworkKind) bool { return false }
func (Record) OwnsProxy(Observation, Binding) bool               { return false }

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
