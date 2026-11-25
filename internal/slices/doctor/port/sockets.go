package port

import (
	"context"

	sockets "github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Sockets detects, afresh on each call, the checkout socket backend the host
// offers for the validated checkout root. It reads the kernel and the selected
// engine and changes nothing. A typed failure carries no backend.
type Sockets interface {
	Detect(ctx context.Context, checkout string, engine readiness.EngineIdentity) sockets.Selection
}
