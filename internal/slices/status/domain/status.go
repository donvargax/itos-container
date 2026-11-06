// Package domain validates project-scoped read-only status requests.
package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func Inspect(ctx context.Context, service port.Recovery, args []string) port.RecoveryResult {
	if len(args) != 0 {
		return port.RecoveryResult{Failures: []port.RecoveryFailure{{Outcome: port.Usage}}}
	}
	return service.Status(ctx)
}
