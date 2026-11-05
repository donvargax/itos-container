// Package porttest provides I/O-free approval fixtures.
package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"strconv"
	"sync"
)

// Store retains independent values and serializes compare-and-publish in memory.
type Store struct {
	mu            sync.Mutex
	Records       map[string]port.Record
	Failure       port.Failure
	CommitFailure port.Failure
	Writes        int
}

// Read exposes only validated supplied fixture state and creates nothing.
func (s *Store) Read(c port.Candidate) (port.Record, port.Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Records[c.Project], s.Failure
}

// Commit compares the active generation before replacing any fixture record.
func (s *Store) Commit(c port.Candidate, generation string) (port.Record, port.Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != "" {
		return port.Record{}, s.Failure
	}
	if s.CommitFailure != "" {
		return port.Record{}, s.CommitFailure
	}
	old := s.Records[c.Project]
	if old.Generation != generation {
		return port.Record{}, "approve.conflict"
	}
	if old.Generation != "" && old.Canonical == c.Canonical {
		return old, ""
	}
	n := uint64(0)
	if generation != "" {
		var err error
		n, err = strconv.ParseUint(generation, 10, 64)
		if err != nil || n == ^uint64(0) {
			return port.Record{}, "state.invalid"
		}
	}
	record := port.Record{Candidate: c, Generation: strconv.FormatUint(n+1, 10)}
	if s.Records == nil {
		s.Records = map[string]port.Record{}
	}
	s.Records[c.Project] = record
	s.Writes++
	return record, ""
}

// Select verifies and copies one current record while holding the same lock.
func (s *Store) Select(c port.Candidate, generation string) (port.Frozen, port.Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != "" {
		return port.Frozen{}, s.Failure
	}
	record := s.Records[c.Project]
	if record.Generation == "" {
		return port.Frozen{}, "missing"
	}
	if record.Generation != generation {
		return port.Frozen{}, "superseded"
	}
	if record.Canonical != c.Canonical || record.Fingerprint != c.Fingerprint {
		return port.Frozen{}, "changed"
	}
	return port.Frozen{Project: record.Project, Generation: record.Generation, Fingerprint: record.Fingerprint, Settings: record.Canonical}, ""
}
