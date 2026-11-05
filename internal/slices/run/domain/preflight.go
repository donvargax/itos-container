package domain

import (
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/slices/run/port"
)

// Preflight is the run selection-to-durable-handoff boundary.
func Preflight(a port.Approval, snapshots approval.Snapshots) (approval.Durable, error) {
	return approval.Durable{}, CheckApproval(a)
}
