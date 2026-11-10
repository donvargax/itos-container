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
type Credentials struct {
	FS      port.CredentialFS
	Input   port.CredentialInput
	Account approval.Account
}

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
