package domain

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func recordMountDiagnostic(ctx context.Context, d port.MountDiagnostic) {
	if state, ok := ctx.Value(startupKey{}).(*startupState); ok {
		state.mountDiagnostic = d
	}
}

func anchorRefusal(ctx context.Context, r port.Record, check, reason string) port.Outcome {
	recordMountDiagnostic(ctx, port.MountDiagnostic{Check: check, Reason: reason,
		ExpectedDevice: r.Anchor.Device, ExpectedInode: r.Anchor.Inode})
	return port.MountRefused
}
