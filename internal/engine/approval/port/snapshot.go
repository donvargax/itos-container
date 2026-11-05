package port

import readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"

// StagedReadiness offers a local checkpoint while keeping the checkout pinned.
type StagedReadiness interface {
	InspectBeforeEngine(func(readiness.Result) bool) readiness.Result
}

// Durable is the immutable selected input and its protected material reference.
type Durable struct {
	Frozen    Frozen
	Reference string
}

// Snapshots publishes only independent selected values, never checkout paths.
type Snapshots interface {
	Publish(Frozen) (Durable, Failure)
}
