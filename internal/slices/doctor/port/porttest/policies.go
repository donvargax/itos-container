// Package porttest supplies an in-memory inspection fake and its shared contract.
package porttest

import policy "github.com/donvargax/itos-container/internal/engine/policy/port"

// Policies supplies a policy inspection result and records inspection calls.
type Policies struct {
	Result policy.Result
	Reads  int
}

// Inspect returns the configured inspection outcome.
func (p *Policies) Inspect() policy.Result { p.Reads++; return p.Result }
