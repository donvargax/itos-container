package porttest

import "github.com/donvargax/itos-container/internal/engine/approval/port"

// Snapshots retains independently selected values and durability observations.
type Snapshots struct {
	Values  map[string]port.Frozen
	Failure string
	Events  []string
}

func (s *Snapshots) Publish(port.Frozen) (port.Durable, port.Failure) {
	return port.Durable{}, "snapshot.write"
}
