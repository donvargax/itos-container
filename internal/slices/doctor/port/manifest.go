package port

import manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"

// Manifests validates one policy-selected file. It has no engine/readiness input.
type Manifests interface {
	Inspect(path string) manifest.Result
}
