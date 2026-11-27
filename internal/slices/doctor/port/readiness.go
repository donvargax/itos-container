package port

import (
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Readiness inspects the selected checkout, engine and cached image only.
type Readiness interface{ Inspect() readiness.Result }

// Approval reads approval state for the readiness inspection doctor already performed.
type Approval interface {
	InspectReadiness(readiness.Result) approval.View
}
