package port

import "context"

const (
	HomeRefused     Outcome = "home-refused"
	HomeUnavailable Outcome = "home-unavailable"
	HomeInUse       Outcome = "home-in-use"
	// HomeNotIssued reports a create the adapter or its finite wrapper refused
	// or skipped before any engine request was sent. Such a failure cancels
	// the caller's reservation; every other failure may have been issued and
	// keeps its uncertainty.
	HomeNotIssued   Outcome = "home-not-issued"
	ConsentRequired Outcome = "consent-required"
)

// HomeReference is recorded with the session before any allocation request.
// Created is Docker's observed creation time, not an invented immutable ID.
type HomeReference struct {
	Scope   string `json:"scope"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created string `json:"created"`
}

type HomeEngineIdentity struct {
	Endpoint string `json:"endpoint"`
	ID       string `json:"id"`
}
type HomeClaim struct {
	Session   string `json:"session"`
	StateRoot string `json:"state_root"`
	Intent    string `json:"intent"`
}
type HomeGeneration struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Created     string      `json:"created"`
	Stage       string      `json:"stage"`
	Initialized bool        `json:"initialized"`
	Successor   string      `json:"successor"`
	Claims      []HomeClaim `json:"claims"`
}
type HomeScope struct {
	Schema      int                `json:"schema"`
	Scope       string             `json:"scope"`
	Project     string             `json:"project"`
	UID         string             `json:"uid"`
	GID         string             `json:"gid"`
	Engine      HomeEngineIdentity `json:"engine"`
	Current     string             `json:"current"`
	Generations []HomeGeneration   `json:"generations"`
}

type HomeCodec interface {
	Scope(Binding) string
	EncodeHome(HomeScope) []byte
	DecodeHome([]byte) (HomeScope, Outcome)
}
type HomeStore interface {
	Scope(Binding) string
	List(context.Context) ([]string, Outcome)
	Open(context.Context, string, bool) (HomeTransaction, Outcome)
	Claim(Record, string) (HomeClaim, Outcome)
	CheckClaims(context.Context, HomeScope, HomeGeneration) Outcome
}

// The scope lock serializes generation selection, durable claims and retirement.
// No project-state transaction is held by this handle.
type HomeTransaction interface {
	Read() (HomeScope, Outcome)
	Save(HomeScope) Outcome
	Close()
}
type HomeObservation struct {
	Name, Created, Driver, Scope, Token, Key string
	ExactLabels, EmptyOptions                bool
}
type HomeConnection interface {
	// AllocateHome returns HomeNotIssued only when no engine request was sent
	// and HomeUnavailable when one may have been; it never reports success
	// for a request whose reply was lost.
	AllocateHome(context.Context, HomeReference) Outcome
	InspectHome(context.Context, string) (HomeObservation, Inspection)
	InitializeHome(context.Context, string, string, string) Outcome
	VerifyHomeRoot(context.Context, string, string, string) Outcome
	RemoveHome(context.Context, string) Outcome
}

func (h HomeReference) Valid() bool {
	return HexID(h.Scope) && HexID(h.ID) && h.Name == "itos-container-home-"+h.ID
}
func (h HomeReference) Owns(o HomeObservation) bool {
	return h.Valid() && o.Name == h.Name && o.Created != "" && (h.Created == "" || h.Created == o.Created) &&
		o.Driver == "local" && o.Scope == "local" && o.ExactLabels && o.EmptyOptions && o.Token == h.ID && o.Key == h.Scope
}
func (g HomeGeneration) Reference(scope string) HomeReference {
	return HomeReference{Scope: scope, ID: g.ID, Name: g.Name, Created: g.Created}
}
func (s HomeScope) Binding() Binding {
	return Binding{UID: s.UID, GID: s.GID, Endpoint: s.Engine.Endpoint, EngineID: s.Engine.ID}
}

type HomeDisclosure struct {
	Storage string `json:"storage"`
	State   string `json:"state"`
	Name    string `json:"name"`
}
type HomeEntry struct {
	ID          string             `json:"id"`
	Scope       string             `json:"scope"`
	Project     string             `json:"project"`
	UID         string             `json:"uid"`
	GID         string             `json:"gid"`
	Engine      HomeEngineIdentity `json:"engine"`
	Name        string             `json:"name"`
	State       string             `json:"state"`
	Disposition string             `json:"disposition"`
	Successor   *string            `json:"successor"`
	References  []string           `json:"references"`
	Retry       []string           `json:"retry"`
}
type HomeResult struct {
	Homes    []HomeEntry
	Failures []RecoveryFailure
}
type HomeConsent interface {
	Review(context.Context, HomeEntry, string) bool
}
type HomeManagement interface {
	ListHomes(context.Context) HomeResult
	Retire(context.Context, string, string, HomeConsent) HomeResult
}
type HomePresentation interface{ PrintHomes(HomeResult, bool) int }

func (r Record) homeTransition(next Record) bool {
	if r.Home == next.Home {
		return true
	}
	if r.Home == (HomeReference{}) {
		return r.Stage == "reserved" && next.Stage == "reserved" && next.Home.Valid()
	}
	expected := r.Home
	expected.Created = next.Home.Created
	return r.Home.Created == "" && next.Home.Created != "" && next.Home == expected
}
func (r Record) ValidHome(b Binding) bool {
	if r.Home == (HomeReference{}) {
		return !b.Home || r.Stage == "reserved" || r.Stage == "cancelled"
	}
	return b.Home && r.Home.Valid() && (r.Home.Created != "" || r.Stage == "reserved" || r.Stage == "cancelled")
}
