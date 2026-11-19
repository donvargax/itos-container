// Package domain validates an exact session lookup before any recovery I/O.
package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func Stop(ctx context.Context, service port.Recovery, args []string) port.RecoveryResult {
	if len(args) != 1 || !port.HexID(args[0]) {
		return port.RecoveryResult{Failures: []port.RecoveryFailure{{Outcome: port.Usage}}}
	}
	return service.Stop(ctx, args[0])
}

func Home(ctx context.Context, service port.HomeManagement, id string, args []string, reset, remove bool, consent port.HomeConsent) port.HomeResult {
	if !port.HexID(id) || len(args) != 0 || reset == remove {
		return port.HomeResult{Failures: []port.RecoveryFailure{{Outcome: port.Usage}}}
	}
	action := "delete"
	if reset {
		action = "reset"
	}
	return service.Retire(ctx, id, action, consent)
}
