package port

import (
	"context"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

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

// PreflightCodec compares only locally observed canonical fields. Engine/image
// facts remain absent until fresh inspection; approval cannot supply them.
type PreflightCodec interface {
	Codec
	LocalCandidate(readiness.Result, string, string) Candidate
	LocalMatches(Candidate, Record) bool
	// ChangedPaths returns sorted pointers only; local omits unobserved engine/image facts.
	ChangedPaths(candidate Candidate, record Record, local bool) []string
}

// FrozenRecords validates the same envelope from independent selected bytes.
type FrozenRecords interface {
	Records
	FrozenRecord(Frozen) (Record, Failure)
}

// ContextReadiness keeps aggregate inspection under the caller's startup scope.
type ContextReadiness interface {
	InspectBeforeEngineContext(context.Context, func(readiness.Result) bool) readiness.Result
}
type ContextPreflight interface {
	InspectContext(context.Context) View
	FreezeContext(context.Context, View) (Frozen, Failure)
}
