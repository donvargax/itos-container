package port

import (
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Credentials observes only source metadata and its protected boundaries.
// Pin holds the effective root before its policy is read, so the requests and
// the checkout they are excluded from belong to one tree. A refusal is a shared
// boundary issue and leaves nothing pinned; Close releases a successful pin.
type Credentials interface {
	Pin() readiness.CredentialResult
	Inspect([]policy.Credential) readiness.CredentialResult
	Close()
}
