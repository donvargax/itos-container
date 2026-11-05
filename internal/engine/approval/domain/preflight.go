package domain

import "github.com/donvargax/itos-container/internal/engine/approval/port"

// Preflight composes run's prerequisite inspection without consent capability.
type Preflight struct {
	Service
	Local port.StagedReadiness
}

// Inspect is the unavailable staged implementation, retained for red-first probes.
func (s Preflight) Inspect() port.View { return s.Service.Inspect() }
