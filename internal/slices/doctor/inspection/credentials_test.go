package inspection

import (
	"path/filepath"
	"testing"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	approvalfake "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	readinessfake "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	doctor "github.com/donvargax/itos-container/internal/slices/doctor/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
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

// The adapter pins Input.Root itself with the shared checkout pin: every
// refusal the pin can give keeps its shared rule and the root as its subject.
func TestCredentialPinContract(t *testing.T) {
	failures := map[string]port.Failure{"checkout.root": port.CheckoutRoot, "checkout.unavailable": port.CheckoutUnavailable, "checkout.path": port.CheckoutPath, "checkout.changed": port.CheckoutChanged}
	porttest.CredentialPinContract(t, func(rule string) (doctor.Credentials, func() bool) {
		checkout := &readinessfake.Checkout{Failure: failures[rule]}
		return Credentials{Input: port.CredentialInput{Root: "/checkout"}, Checkout: checkout}, func() bool { return checkout.Opened }
	})
}

func TestCredentialPinNamesOnlyTheEffectiveRoot(t *testing.T) {
	checkout := &readinessfake.Checkout{}
	c := Credentials{Input: port.CredentialInput{Root: "/projects/checkout", State: "/state"}, Checkout: checkout}
	if got := c.Pin(); len(got.Issues) != 0 || len(checkout.Roots) != 1 || checkout.Roots[0] != "/projects/checkout" {
		t.Fatalf("pinned %v: %#v", checkout.Roots, got)
	}
	// A failure the pin does not name is unavailable evidence, never a pass.
	for _, failure := range []port.Failure{port.CheckoutLayout, port.CheckoutHardlink, port.EngineUnavailable} {
		c.Checkout = &readinessfake.Checkout{Failure: failure}
		want := port.CredentialIssue{Rule: "checkout.unavailable", Index: -1, Subject: "/projects/checkout"}
		if got := c.Pin(); len(got.Issues) != 1 || got.Issues[0] != want {
			t.Fatalf("failure %d: %#v", failure, got)
		}
	}
}
