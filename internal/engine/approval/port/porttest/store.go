// Package porttest provides I/O-free approval fixtures.
package porttest

import "github.com/donvargax/itos-container/internal/engine/approval/port"

// Store retains fixture records and injected storage failures without I/O.
type Store struct {
	Records map[string]port.Record
	Failure port.Failure
	Writes  int
}

// Read exposes supplied fixture state.
func (s *Store) Read(c port.Candidate) (port.Record, port.Failure) {
	return s.Records[c.Project], s.Failure
}

// Commit is an unavailable placeholder until the red approval contracts are implemented.
func (*Store) Commit(port.Candidate, string) (port.Record, port.Failure) {
	return port.Record{}, "unimplemented"
}

// Select is an unavailable placeholder until the red selection contracts are implemented.
func (*Store) Select(port.Candidate, string) (port.Frozen, port.Failure) {
	return port.Frozen{}, "unimplemented"
}
