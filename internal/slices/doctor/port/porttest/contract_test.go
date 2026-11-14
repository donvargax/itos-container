package porttest_test

import (
	"reflect"
	"testing"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestPoliciesContract(t *testing.T) {
	porttest.PoliciesContract(t, func(f porttest.Fixture) port.Policies {
		return &porttest.Policies{Result: f.Result}
	})
}

func TestManifestsContract(t *testing.T) {
	porttest.ManifestsContract(t, func(f porttest.ManifestFixture) port.Manifests { return &porttest.Manifests{Result: f.Result} })
}

func TestCredentialPinContract(t *testing.T) {
	porttest.CredentialPinContract(t, func(rule string) (port.Credentials, func() bool) {
		fake := &porttest.Credentials{}
		if rule != "" {
			fake.Refusal.Issues = []readiness.CredentialIssue{{Rule: rule, Index: -1, Subject: "/checkout"}}
		}
		return fake, func() bool { return fake.Pinned }
	})
}

func TestRootPoliciesReadOnlyThroughAHeldPin(t *testing.T) {
	root := &porttest.Credentials{}
	want := policy.Result{Policy: &policy.Policy{Version: 3, Manifest: "a.json"}}
	policies := &porttest.Policies{Result: want, Root: root}
	unreadable := policy.Result{Issues: []policy.Issue{{Kind: policy.Unreadable}}}
	if got := policies.Inspect(); !reflect.DeepEqual(got, unreadable) {
		t.Fatalf("read before the pin = %#v", got)
	}
	root.Pin()
	if got := policies.Inspect(); !reflect.DeepEqual(got, want) {
		t.Fatalf("read through the pin = %#v", got)
	}
	root.Close()
	if got := policies.Inspect(); !reflect.DeepEqual(got, unreadable) {
		t.Fatalf("read after the release = %#v", got)
	}
	if events := []string{"policy", "pin", "policy", "close", "policy"}; !reflect.DeepEqual(root.Events, events) || policies.Reads != 3 {
		t.Fatalf("events %v, reads %d", root.Events, policies.Reads)
	}
}
