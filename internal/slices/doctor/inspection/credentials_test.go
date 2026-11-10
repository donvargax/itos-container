package inspection

import (
	"path/filepath"
	"testing"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	approvalfake "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestCredentialAssemblyReservesAccountHome(t *testing.T) {
	accountHome := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, tc := range []struct {
		home    string
		failure approval.Failure
		want    string
	}{
		{accountHome, "", filepath.Join(accountHome, ".local", "state", "itos-container")},
		{accountHome, "state.unavailable", ""}, {"", "", ""}, {"relative", "", ""},
		{string(filepath.Separator), "", ""}, {accountHome + "\xff", "", ""},
	} {
		input := port.CredentialInput{State: "selected", AccountState: "must not survive failure"}
		resolved := credentialAccountInput(input, &approvalfake.Account{Home: tc.home, Failure: tc.failure})
		if resolved.AccountState != tc.want || resolved.State != input.State {
			t.Fatalf("account input %#v: %#v", tc, resolved)
		}
	}
}

func TestCredentialNoGrantsDoesNotObserveAccount(t *testing.T) {
	// Neither port may be touched for the empty request.
	if got := (Credentials{}).Inspect(nil); len(got.Issues) != 0 {
		t.Fatal(got)
	}
}
