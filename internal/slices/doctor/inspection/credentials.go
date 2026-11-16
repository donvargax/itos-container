package inspection

import (
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Credentials observes the fixed account anchor only after policy validation
// requests source inspection. File-only scopes and empty grants never query it.
// Checkout is the shared pin the scope's policy reader opens through: assembly
// gives both the same value, so the policy comes from Input.Root's own tree.
type Credentials struct {
	Input    port.CredentialInput
	Checkout port.Checkout
	Observer port.CredentialObserver
}

// A pin failure outside this table is unavailable evidence, never a pass.
var credentialPinRules = map[port.Failure]string{
	port.CheckoutRoot:    "checkout.root",
	port.CheckoutPath:    "checkout.path",
	port.CheckoutChanged: "checkout.changed",
}

// Pin holds Input.Root, never the working directory, before the policy is read.
func (c Credentials) Pin() port.CredentialResult {
	if len(c.Input.Root) > 4095 {
		return port.CredentialResult{Issues: []port.CredentialIssue{{Rule: "checkout.root", Index: -1, Subject: c.Input.Root}}}
	}
	failure := c.Checkout.Pin(c.Input.Root)
	if failure == port.None {
		return port.CredentialResult{}
	}
	rule, known := credentialPinRules[failure]
	if !known {
		rule = "checkout.unavailable"
	}
	return port.CredentialResult{Issues: []port.CredentialIssue{{Rule: rule, Index: -1, Subject: c.Input.Root}}}
}

// Close releases the pin once the policy and its sources have been observed.
func (c Credentials) Close() { c.Checkout.Close() }

func (c Credentials) Inspect(grants []policy.Credential) port.CredentialResult {
	if len(grants) == 0 {
		return port.CredentialResult{}
	}
	return c.Observer.Inspect(grants)
}
