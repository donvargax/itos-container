package porttest

import (
	"bytes"
	"context"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Stage stages nothing yet: the staged log's tests are committed first, red.
func (s *Registry) Stage(context.Context, port.Candidate, string) (port.StagedLog, port.Failure) {
	return nil, ""
}

// SessionLog returns a session's published log, nil when none was published,
// and the number of its handles staged and not yet released.
func (s *Registry) SessionLog(project, id string) ([]byte, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.Sessions[project+"/"+id]
	if f == nil {
		return nil, 0
	}
	return bytes.Clone(f.Log), f.Staged
}
