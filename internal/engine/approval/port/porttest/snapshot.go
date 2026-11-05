package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"sync"
)

// Snapshots models immutable publication, including a complete but unconfirmed
// record after a directory-sync failure. It performs no I/O.
type Snapshots struct {
	mu      sync.Mutex
	Values  map[string]port.Frozen
	Failure string
	Events  []string
}

func (s *Snapshots) Publish(f port.Frozen) (port.Durable, port.Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := f.Project + "/" + f.Generation
	refusal := port.Durable{Reference: "snapshots/" + key + ".json"}
	if s.Values == nil {
		s.Values = map[string]port.Frozen{}
	}
	old, exists := s.Values[key]
	if exists && old != f {
		return refusal, "snapshot.invalid"
	}
	if !exists {
		for _, event := range []string{"new ancestor fsync", "private file creation", "short write", "file fsync", "rename"} {
			s.Events = append(s.Events, event)
			if s.Failure == event {
				return refusal, "snapshot.write"
			}
		}
		s.Values[key] = f
	}
	for _, event := range []string{"file fsync", "parent directory fsync", "new ancestor fsync"} {
		s.Events = append(s.Events, event)
		if s.Failure == event {
			return refusal, "snapshot.write"
		}
	}
	return port.Durable{Frozen: f, Reference: refusal.Reference}, ""
}
