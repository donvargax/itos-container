// Package port defines owned, unstarted resource operations and durable evidence.
package port

import (
	"context"
	"strings"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type Outcome string

const (
	Owned           Outcome = "owned-unstarted"
	Complete        Outcome = "cleanup-complete"
	IntentWrite     Outcome = "intent-write-failed"
	OwnershipWrite  Outcome = "ownership-write-failed"
	CompletionWrite Outcome = "completion-write-failed"
	Unsafe          Outcome = "ownership-unsafe"
	Invalid         Outcome = "ownership-invalid"
	Refused         Outcome = "ownership-refused"
	IdentityFailed  Outcome = "intent-identity-failed"
	Uncertain       Outcome = "create-uncertain"
	Incomplete      Outcome = "cleanup-incomplete"
)

// A failed result never carries an ID usable as a startup handoff. Cleanup is
// separate so a persistence error survives even when compensating removal works.
type Result struct {
	Outcome       Outcome
	Reference, ID string
	Cleanup       Outcome
}

type Record struct {
	Schema         int    `json:"schema"`
	Project        string `json:"project"`
	Generation     string `json:"generation"`
	Fingerprint    string `json:"fingerprint"`
	Snapshot       string `json:"snapshot"`
	UID            string `json:"uid"`
	GID            string `json:"gid"`
	Endpoint       string `json:"engine_endpoint"`
	EngineID       string `json:"engine_id"`
	Name           string `json:"name"`
	ImageReference string `json:"image_reference"`
	ImageID        string `json:"image_id"`
	ContainerID    string `json:"container_id"`
	Stage          string `json:"stage"`
}

type Binding struct {
	Frozen                                                approval.Frozen
	Checkout                                              readiness.CheckoutIdentity
	UID, GID, Endpoint, EngineID, ImageReference, ImageID string
	Workspace, Environment                                string
}

func (b Binding) Intent(token string) Record {
	return Record{Schema: 1, Project: b.Frozen.Project, Generation: b.Frozen.Generation, Fingerprint: b.Frozen.Fingerprint,
		Snapshot: "snapshots/" + b.Frozen.Project + "/" + b.Frozen.Generation + ".json", UID: b.UID, GID: b.GID,
		Endpoint: b.Endpoint, EngineID: b.EngineID, Name: "itos-container-" + token, ImageReference: b.ImageReference, ImageID: b.ImageID, Stage: "intent"}
}

// HexID is the complete lowercase 256-bit identity, never an engine short ID.
func HexID(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}
func Reference(project, token string) string { return "intents/" + project + "/" + token + ".json" }
func ParseReference(ref string) (project, token string, ok bool) {
	parts := strings.Split(ref, "/")
	if len(parts) != 3 || parts[0] != "intents" || !strings.HasSuffix(parts[2], ".json") {
		return "", "", false
	}
	project, token = parts[1], strings.TrimSuffix(parts[2], ".json")
	return project, token, HexID(project) && HexID(token)
}

type Codec interface {
	Encode(Record) []byte
	Decode([]byte) (Record, Outcome)
	Frozen(approval.Frozen) (Binding, Outcome)
	Snapshot([]byte) (Binding, Outcome)
}
type Entropy interface{ Token() (string, Outcome) }

// Store transactions retain the protected lock until Close. Validate checks the
// original snapshot and actual host identity, never current startup inputs.
type Store interface {
	New(context.Context, approval.Durable, string) (Intent, Outcome)
	Open(context.Context, string) (Intent, Outcome)
}
type Intent interface {
	Record() Record
	Binding() Binding
	Validate() Outcome
	Save(Record) Outcome
	Close()
}

type Observation struct {
	ID, Name, ImageID           string
	Token, Project, Fingerprint string
	StatePresent                bool
	Status                      string
	Running                     bool
	PID                         int
	Started                     bool
	Mounts, Volumes             bool // Any materialized mount or declared volume.
}
type Inspection uint8

const (
	Observed Inspection = iota
	NotFound
	Unavailable
	Changed
)

// Open pins the selected native socket; Check refreshes daemon identity without
// renewing the caller's deadline. No capability here can start or acquire work.
type Engine interface {
	Open(context.Context, Binding) (Connection, Outcome)
}
type Connection interface {
	Check(context.Context) Outcome
	// Create inspects the frozen image ID and excludes image-declared volumes
	// before mutation. Refused means no create was sent; Uncertain is not absence.
	Create(context.Context, Record) (string, Outcome)
	Inspect(context.Context, string) (Observation, Inspection)
	Remove(context.Context, string)
	Close()
}

func (r Record) NameToken() string {
	if !strings.HasPrefix(r.Name, "itos-container-") {
		return ""
	}
	return strings.TrimPrefix(r.Name, "itos-container-")
}
