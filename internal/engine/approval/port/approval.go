// Package port owns approval identities, canonical values and protected state capabilities.
package port

import readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"

// Failure is a stable internal refusal, independent of CLI presentation.
type Failure string

// Candidate is a complete validated canonical value. Strings give callers no
// mutable buffer that could change an already selected approval.
type Candidate struct {
	UID, GID, Root, Project, Fingerprint, Canonical string
}

// Record is the only active approval for a project.
type Record struct {
	Candidate
	Generation string
}

// Frozen carries an independent selected value, never a path to checkout data.
type Frozen struct {
	Project, Generation, Fingerprint string
	Settings                         string
}

// Bytes returns a fresh copy for each consumer.
func (f Frozen) Bytes() []byte { return []byte(f.Settings) }

// Codec supplies canonical JSON and SHA-256 outside domain code.
type Codec interface {
	Candidate(readiness.Result, string, string) Candidate
}

// Store validates records and serializes comparisons and durable publication.
// Read never creates state. Commit compares the generation captured for review.
// Select verifies the candidate and expected generation atomically.
type Store interface {
	Read(Candidate) (Record, Failure)
	Commit(Candidate, string) (Record, Failure)
	Select(Candidate, string) (Frozen, Failure)
}

// Readiness inspects only the existing strict file/host/engine prerequisites.
type Readiness interface{ Inspect() readiness.Result }

// Host supplies effective IDs and a socket identity observation. Neither is
// taken from environment variables or from image Config.User.
type Host interface {
	Identity() (string, string)
	Socket(string) string
}

// Difference retains JSON values as canonical text for exact safe presentation.
type Difference struct{ Path, Before, After string }

// View combines the candidate, readable active record and typed refusal.
type View struct {
	Inspection    readiness.Result
	Candidate     Candidate
	Record        Record
	Status        string
	Failure       Failure
	Field, Socket string
	Differences   []Difference
	ChangedPaths  []string
}

// Inspector is read-only; Approver rechecks a reviewed candidate after consent.
type Inspector interface{ Inspect() View }
type Approver interface {
	Inspector
	Approve(View) View
}

// Records encodes and validates the complete canonical storage envelope.
type Records interface {
	Encode(Record) []byte
	Decode(Candidate, []byte) (Record, Failure)
}

// Problem and Report are presentation values; raw rejected input is never a subject.
type Problem = readiness.Problem
type Change struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}
type Report struct {
	Status      string   `json:"status"`
	Fingerprint string   `json:"fingerprint"`
	Effective   any      `json:"effective"`
	Generation  *string  `json:"generation"`
	Differences []Change `json:"differences"`
	// EffectiveText is terminal-safe JSON for plain display only.
	EffectiveText string `json:"-"`
}

// Presentation owns terminal I/O and structured rendering behind a port.
type Reviewer interface {
	Terminal() bool
	Review(View) bool
}

// Presentation also exposes structured and plain rendering.
type Presentation interface {
	Reviewer
	Report(View) Report
	Problems(View) []Problem
	Print(View, bool) int
}

// RequiresTerminal identifies grants whose consent cannot use the fingerprint route.
// The policy has already passed readiness validation when consent is considered.
func (v View) RequiresTerminal() bool {
	return v.Inspection.Policy != nil && v.Inspection.Policy.Policy != nil && v.Inspection.Policy.Policy.Network.Mode == "allowlist"
}
