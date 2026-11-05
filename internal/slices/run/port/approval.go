// Package port declares run's read-only approval prerequisite.
package port

import approval "github.com/donvargax/itos-container/internal/engine/approval/port"

// Approval cannot publish or request consent.
type Approval interface {
	Inspect() approval.View
	Freeze(approval.View) (approval.Frozen, approval.Failure)
}
