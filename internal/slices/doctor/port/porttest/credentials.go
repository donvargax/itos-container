package porttest

import (
	"reflect"
	"slices"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// CredentialPinContract runs against the fake and the inspection adapter. open
// returns sources whose root is "/checkout" and either pins or refuses with the
// named shared rule, beside a probe saying whether that root is held now.
func CredentialPinContract(t Reporter, open func(rule string) (port.Credentials, func() bool)) {
	t.Helper()
	for _, rule := range []string{"", "checkout.root", "checkout.unavailable", "checkout.path", "checkout.changed"} {
		sources, held := open(rule)
		want := readiness.CredentialResult{}
		if rule != "" {
			want.Issues = []readiness.CredentialIssue{{Rule: rule, Index: -1, Subject: "/checkout"}}
		}
		if got := sources.Pin(); !reflect.DeepEqual(got, want) {
			t.Errorf("Pin() with %q = %#v, want %#v", rule, got, want)
		}
		if held() != (rule == "") {
			t.Errorf("Pin() with %q left the root held = %t", rule, held())
		}
		if rule != "" {
			continue
		}
		sources.Close()
		if held() {
			t.Errorf("Close() left the root held")
		}
	}
}

// Credentials supplies an observation result and retains independent requests.
// Events records the order of Pin, Inspect and Close; Pinned follows the pin.
type Credentials struct {
	Result  readiness.CredentialResult
	Refusal readiness.CredentialResult
	Calls   [][]policy.Credential
	Events  []string
	Pinned  bool
}

// Pin holds the root unless a refusal is configured; a refusal pins nothing.
func (c *Credentials) Pin() readiness.CredentialResult {
	c.Events = append(c.Events, "pin")
	c.Pinned = len(c.Refusal.Issues) == 0
	return readiness.CredentialResult{Issues: slices.Clone(c.Refusal.Issues)}
}

func (c *Credentials) Inspect(grants []policy.Credential) readiness.CredentialResult {
	c.Events = append(c.Events, "inspect")
	c.Calls = append(c.Calls, slices.Clone(grants))
	return readiness.CredentialResult{Issues: slices.Clone(c.Result.Issues)}
}

func (c *Credentials) Close() {
	c.Events = append(c.Events, "close")
	c.Pinned = false
}
