// Package porttest supplies an in-memory inspection fake and its shared contract.
package porttest

import policy "github.com/donvargax/itos-container/internal/engine/policy/port"

// Policies supplies a policy inspection result and records inspection calls.
// With Root set it reads through that fake's pin, as the credentials scope's
// reader does: each read joins the pin's event order, and a read outside a held
// pin is unreadable rather than an answer from some other location.
type Policies struct {
	Result policy.Result
	Reads  int
	Root   *Credentials
}

// Inspect returns the configured inspection outcome.
func (p *Policies) Inspect() policy.Result {
	p.Reads++
	if p.Root == nil {
		return p.Result
	}
	p.Root.Events = append(p.Root.Events, "policy")
	if !p.Root.Pinned {
		return policy.Result{Issues: []policy.Issue{{Kind: policy.Unreadable}}}
	}
	return p.Result
}
