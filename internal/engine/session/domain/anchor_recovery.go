package domain

import (
	"context"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type recoveryAnchorKey struct{}
type recoveryAnchorState struct {
	session port.Session
	entry   *port.RecoveryEntry
}

func anchorAuthority(ctx context.Context, i port.Intent, c port.RunConnection) port.Outcome {
	if budget.Cause(ctx) != nil {
		return port.Incomplete
	}
	if state, ok := ctx.Value(recoveryAnchorKey{}).(recoveryAnchorState); ok {
		if i.Record().Anchor.Session != state.session.Record().ID {
			return port.Refused
		}
		if f := state.session.Validate(); f != "" {
			return f
		}
	}
	if f := i.Validate(); f != "" {
		return f
	}
	return c.Check(ctx)
}

func inspectRecoveryAnchor(ctx context.Context, i port.Intent, c port.RunConnection) (o port.AnchorObservation, failure port.Outcome) {
	fact := "unknown"
	defer func() {
		if state, ok := ctx.Value(recoveryAnchorKey{}).(recoveryAnchorState); ok {
			state.entry.Anchor = fact
		}
	}()
	if f := anchorAuthority(ctx, i, c); f != "" {
		return o, f
	}
	r := i.Record()
	o, status := c.InspectAnchor(ctx, r.Anchor.Name)
	if status == port.Changed {
		return o, port.Refused
	}
	if status == port.NotFound {
		fact = "absent"
		return o, ""
	}
	if status != port.Observed {
		return o, port.Incomplete
	}
	if !r.OwnsAnchor(o) || r.Anchor.Stage == "absent" {
		return o, port.Refused
	}
	fact = "present"
	return o, ""
}
