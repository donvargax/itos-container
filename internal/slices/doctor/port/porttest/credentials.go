package porttest

import (
	"slices"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Credentials supplies an observation result and retains independent requests.
type Credentials struct {
	Result readiness.CredentialResult
	Calls  [][]policy.Credential
}

func (c *Credentials) Inspect(grants []policy.Credential) readiness.CredentialResult {
	c.Calls = append(c.Calls, slices.Clone(grants))
	return readiness.CredentialResult{Issues: slices.Clone(c.Result.Issues)}
}
