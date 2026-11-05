// Package port declares approve's shared engine and review dependencies.
package port

import approval "github.com/donvargax/itos-container/internal/engine/approval/port"

// Service inspects and publishes reviewed settings without deciding consent.
type Service interface {
	Inspect() approval.View
	Approve(approval.View) approval.View
}
