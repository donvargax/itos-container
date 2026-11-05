package port

import readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"

// Readiness inspects the selected checkout, engine and cached image only.
type Readiness interface{ Inspect() readiness.Result }
