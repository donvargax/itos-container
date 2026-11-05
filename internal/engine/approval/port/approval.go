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
