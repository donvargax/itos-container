// Package porttest supplies in-memory policy inputs and shared port contracts.
package porttest

import "github.com/donvargax/itos-container/internal/engine/policy/port"

// Files supplies policy bytes or a configured read failure.
type Files struct {
	Data    []byte
	Failure port.Failure
}

// Read returns independent bytes, or only the configured failure.
func (f Files) Read() ([]byte, port.Failure) {
	if f.Failure != port.None {
		return nil, f.Failure
	}
	return append([]byte(nil), f.Data...), port.None
}

// Source supplies representation-checked values and records inspection reads.
type Source struct {
	Value  port.Value
	Issues []port.Issue
	Reads  int
}

// Read returns the configured representation outcome.
func (s *Source) Read() (port.Value, []port.Issue) {
	s.Reads++
	if len(s.Issues) != 0 {
		return port.Value{}, s.Issues
	}
	return s.Value, nil
}
