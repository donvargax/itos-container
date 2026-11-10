package port

import (
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Credentials observes only source metadata and its protected boundaries.
type Credentials interface {
	Inspect([]policy.Credential) readiness.CredentialResult
}
