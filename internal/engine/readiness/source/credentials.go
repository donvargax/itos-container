// Package source binds credential observation to the policy pin and host paths.
package source

import (
	"path/filepath"
	"unicode/utf8"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/domain"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type Credentials struct {
	FS       port.CredentialFS
	Input    port.CredentialInput
	Account  approval.Account
	Checkout port.PinnedCheckout
}

// SelectedState preserves XDG selection and the absolute HOME fallback.
func SelectedState(state, home string) string {
	if state == "" && filepath.IsAbs(home) {
		state = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(state) {
		return ""
	}
	return filepath.Join(state, "itos-container")
}

func (c Credentials) Boundary() port.CredentialResult {
	state := c.Input.State
	if len(state) > 4095 {
		return port.CredentialResult{Issues: []port.CredentialIssue{{Rule: "state.location", Index: -1}}}
	}
	return port.CredentialResult{}
}

// Inspect reads no credential data. Each caller gets a fresh account boundary
// and must prove that the independent walk observed its retained policy pin.
func (c Credentials) Inspect(grants []policy.Credential) port.CredentialResult {
	if len(grants) == 0 {
		return port.CredentialResult{}
	}
	input := c.Input
	if c.Checkout == nil {
		return port.CredentialResult{Issues: []port.CredentialIssue{{Rule: "checkout.unavailable", Index: -1, Subject: input.Root}}}
	}
	pin, failure := c.Checkout.PinnedIdentity()
	if failure != port.None {
		return port.CredentialResult{Issues: []port.CredentialIssue{{Rule: "checkout.unavailable", Index: -1, Subject: input.Root}}}
	}
	input.PolicyDevice, input.PolicyInode = pin.Device, pin.Inode
	input = AccountInput(input, c.Account)
	return (domain.Credentials{FS: c.FS, Input: input}).Inspect(grants)
}

// AccountInput resolves the fixed control tree independently of HOME and XDG.
func AccountInput(input port.CredentialInput, account approval.Account) port.CredentialInput {
	home, failure := account.AccountHome()
	input.AccountState = ""
	if failure == "" && filepath.IsAbs(home) && home != string(filepath.Separator) && utf8.ValidString(home) {
		input.AccountState = filepath.Join(home, ".local", "state", "itos-container")
	}
	return input
}
