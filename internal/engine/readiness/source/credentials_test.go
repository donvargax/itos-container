package source

import (
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	approvalfake "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	fake "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	"path/filepath"
	"strings"
	"testing"
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
		resolved := AccountInput(input, &approvalfake.Account{Home: tc.home, Failure: tc.failure})
		if resolved.AccountState != tc.want || resolved.State != input.State {
			t.Fatalf("account input %#v: %#v", tc, resolved)
		}
	}
}

func TestCredentialObservationRequiresThePolicyPin(t *testing.T) {
	grants := []policy.Credential{{Kind: "claude-token", Source: "/vault/token"}}
	for _, state := range []string{"matching", "missing capability", "closed", "device mismatch", "inode mismatch"} {
		fs := fake.CredentialFixture()
		fs.Parser = func([]byte) ([]port.CredentialMount, bool) {
			return []port.CredentialMount{{ID: 1, Device: 1, Root: "/", Point: "/", Filesystem: "ext4"}}, true
		}
		pin := &fake.Checkout{Identity: port.CheckoutIdentity{Root: "/checkout", Device: "1", Inode: "2"}}
		_ = pin.Pin("/checkout")
		c := Credentials{FS: fs, Checkout: pin, Input: port.CredentialInput{Root: "/checkout", State: "/state", OS: "linux", Architecture: "amd64", UID: 1000}, Account: &approvalfake.Account{Home: "/account"}}
		want := ""
		switch state {
		case "missing capability":
			c.Checkout = nil
			want = "checkout.unavailable"
		case "closed":
			pin.Close()
			want = "checkout.unavailable"
		case "device mismatch":
			pin.Identity.Device = "2"
			want = "checkout.changed"
		case "inode mismatch":
			pin.Identity.Inode = "3"
			want = "checkout.changed"
		}
		got := c.Inspect(grants)
		if want == "" {
			if len(got.Issues) != 0 {
				t.Fatalf("matching policy pin: %#v", got)
			}
		} else if len(got.Issues) != 1 || got.Issues[0].Rule != want || got.Issues[0].Subject != "/checkout" || fs.Calls["source metadata"] != 0 {
			t.Fatalf("%s: %#v %v", state, got, fs.Calls)
		}
	}
	if got := (Credentials{}).Inspect(nil); len(got.Issues) != 0 {
		t.Fatal(got)
	}
}

func TestCredentialBoundaryCountsSelectedStateBytesBeforeObservation(t *testing.T) {
	for _, state := range []string{"/" + strings.Repeat("a", 4095), "/" + strings.Repeat("é", 2048)} {
		got := (Credentials{Input: port.CredentialInput{State: state}}).Boundary()
		if len(got.Issues) != 1 || got.Issues[0].Rule != "state.location" {
			t.Fatalf("invalid boundary accepted: %#v", got)
		}
	}
	state := "/" + strings.Repeat("é", 2047)
	if len(state) != 4095 {
		t.Fatal("bad byte control")
	}
	if got := (Credentials{Input: port.CredentialInput{State: state}}).Boundary(); len(got.Issues) != 0 {
		t.Fatal(got)
	}
	// Short lexical defects retain the store/control refusal order without
	// grants; source inspection still validates them before its filesystem work.
	for _, state := range []string{"relative", "", "/state\xff"} {
		if got := (Credentials{Input: port.CredentialInput{State: state}}).Boundary(); len(got.Issues) != 0 {
			t.Fatal(got)
		}
		pin := &fake.Checkout{Identity: port.CheckoutIdentity{Root: "/checkout", Device: "1", Inode: "2"}}
		_ = pin.Pin("/checkout")
		c := Credentials{Checkout: pin, Input: port.CredentialInput{Root: "/checkout", State: state, OS: "linux", Architecture: "amd64", UID: 1000}, Account: &approvalfake.Account{Home: "/account"}}
		got := c.Inspect([]policy.Credential{{Kind: "claude-token", Source: "/vault/token"}})
		if len(got.Issues) != 1 || got.Issues[0].Rule != "state.location" {
			t.Fatal(got)
		}
	}
}

func TestSelectedStateRetainsSelectionAndFallback(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(home, "selected")
	for _, tc := range []struct{ selected, home, want string }{{state, home, filepath.Join(state, "itos-container")}, {"", home, filepath.Join(home, ".local", "state", "itos-container")}, {"relative", home, ""}, {"", "relative", ""}} {
		if got := SelectedState(tc.selected, tc.home); got != tc.want {
			t.Fatalf("selected state %q, want %q", got, tc.want)
		}
	}
}
