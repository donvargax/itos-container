package inspection

import (
	"path/filepath"
	"unicode/utf8"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/domain"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Credentials observes the fixed account anchor only after policy validation
// requests source inspection. File-only scopes and empty grants never query it.
// Checkout is the shared pin the scope's policy reader opens through: assembly
// gives both the same value, so the policy comes from Input.Root's own tree.
type Credentials struct {
	FS       port.CredentialFS
	Input    port.CredentialInput
	Account  approval.Account
	Checkout port.Checkout
}

// A pin failure outside this table is unavailable evidence, never a pass.
var credentialPinRules = map[port.Failure]string{
	port.CheckoutRoot:    "checkout.root",
	port.CheckoutPath:    "checkout.path",
	port.CheckoutChanged: "checkout.changed",
}

// Pin holds Input.Root, never the working directory, before the policy is read.
func (c Credentials) Pin() port.CredentialResult {
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
	input := c.Input
	if len(grants) > 0 {
		input = credentialAccountInput(input, c.Account)
	}
	return (domain.Credentials{FS: c.FS, Input: input}).Inspect(grants)
}

// AccountHome is the same environment-independent anchor used by control state.
// HOME and XDG_STATE_HOME select ordinary state but cannot move this boundary.
func credentialAccountInput(input port.CredentialInput, account approval.Account) port.CredentialInput {
	home, failure := account.AccountHome()
	input.AccountState = ""
	if failure == "" && filepath.IsAbs(home) && home != string(filepath.Separator) && utf8.ValidString(home) {
		input.AccountState = filepath.Join(home, ".local", "state", "itos-container")
	}
	return input
}
