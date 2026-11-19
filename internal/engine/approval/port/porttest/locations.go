package porttest

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Locations selects independently seeded original protected trees, without I/O.
type Locations struct {
	Stores     map[string]port.Storage
	Registries map[string]*Registry
	Failure    port.Failure
	Changed    bool
}

func (s *Locations) At(root string) port.Storage { return s.Stores[root] }
func (s *Locations) Inspect(ctx context.Context, c port.Candidate, root string) (port.AdmissionDirectory, port.Failure) {
	if s.Failure != "" {
		return nil, s.Failure
	}
	r := s.Registries[root]
	if r == nil {
		return nil, ""
	}
	d, f := r.Admission(ctx, c)
	if d == nil {
		return nil, f
	}
	return locationDirectory{AdmissionDirectory: d, locations: s}, f
}

type locationDirectory struct {
	port.AdmissionDirectory
	locations *Locations
}

func (d locationDirectory) Unchanged() bool {
	return !d.locations.Changed && d.AdmissionDirectory.Unchanged()
}
