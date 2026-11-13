package domain

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func stopAnchoredSession(ctx context.Context, s port.Session, c port.RunConnection, signal string, exit *int, remove, immediate bool) port.Outcome {
	if _, failure := inspectRecoveryAnchor(ctx, s.Intent(), c); failure != "" {
		return failure
	}
	if uncreated(s) {
		// The operation lock fences launch while the terminal no-create proof
		// is published. No empty ID or name-only lookup supplies this proof.
		r := s.Intent().Record()
		r.Stage = "cancelled"
		if failure := anchorAuthority(ctx, s.Intent(), c); failure != "" {
			return failure
		}
		if failure := s.Intent().Save(r); failure != "" {
			return failure
		}
		if state, ok := ctx.Value(recoveryAnchorKey{}).(recoveryAnchorState); ok {
			state.entry.Resource = "absent"
		}
		if failure := releaseSessionResources(ctx, s.Intent(), c); failure != "" {
			return failure
		}
		return cancelUncreated(s)
	}
	if !immediate {
		return stopSession(ctx, s, c, signal, exit, remove)
	}
	// A failed detector is not a graceful cancellation. Kill the independently
	// revalidated worker immediately, without waiting or inventing its exit.
	if exit != nil {
		if failure := recordExit(s, *exit); failure != "" {
			return failure
		}
	}
	r := s.Record()
	r.State = "stopping"
	if failure := s.Save(r); failure != "" {
		return failure
	}
	if failure := recoverySignal(ctx, s, c, "SIGKILL"); failure != "" {
		return failure
	}
	if failure := cleanupRun(ctx, s.Intent(), c); failure != port.Complete {
		return failure
	}
	return finishSession(s)
}

func completeRun(ctx context.Context, i port.Intent, c port.RunConnection, r port.Record, id string) port.Outcome {
	if r.Schema == 1 {
		return complete(i, r, id)
	}
	r.Stage, r.ContainerID = "absent", id
	if failure := i.Save(r); failure != "" {
		return port.CompletionWrite
	}
	if failure := releaseSessionResources(ctx, i, c); failure != "" {
		return failure
	}
	return port.Complete
}

func finishOwnedSession(ctx context.Context, s port.Session, c port.RunConnection) port.Outcome {
	i := s.Intent()
	if i.Record().Schema >= 2 {
		if failure := completeRun(ctx, i, c, i.Record(), i.Record().ContainerID); failure != port.Complete {
			return failure
		}
	}
	return finishSession(s)
}
