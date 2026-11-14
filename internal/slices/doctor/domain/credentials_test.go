package domain_test

import (
	"reflect"
	"testing"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/domain"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestCredentialInspectionStopsAtPolicyFailure(t *testing.T) {
	sources := &porttest.Credentials{}
	for _, result := range []policy.Result{{Issues: []policy.Issue{{Kind: policy.Syntax}}}, {Policy: &policy.Policy{}, Issues: []policy.Issue{{Kind: policy.Type}}}} {
		p, c := domain.InspectCredentials(&porttest.Policies{Result: result}, sources)
		if !reflect.DeepEqual(p, result) || c != nil || len(sources.Calls) != 0 {
			t.Fatalf("policy failure reached sources: %#v %#v", p, c)
		}
	}
	grants := []policy.Credential{{Kind: "github-token", Source: "/private/token"}}
	_, got := domain.InspectCredentials(&porttest.Policies{Result: policy.Result{Policy: &policy.Policy{Credentials: grants}}}, sources)
	if got == nil || len(sources.Calls) != 1 || !reflect.DeepEqual(sources.Calls[0], grants) {
		t.Fatalf("lost requests: %#v %#v", got, sources.Calls)
	}
}

// The policy fake reads through the sources' pin, so a read before the pin or
// after its release would be unreadable and the recorded order would show it.
func TestCredentialInspectionReadsThePolicyThroughAHeldRoot(t *testing.T) {
	grants := []policy.Credential{{Kind: "claude-token", Source: "/private/token"}}
	valid := policy.Result{Policy: &policy.Policy{Version: 3, Credentials: grants}}
	sources := &porttest.Credentials{}
	p, c := domain.InspectCredentials(&porttest.Policies{Result: valid, Root: sources}, sources)
	if !reflect.DeepEqual(p, valid) || c == nil || len(c.Issues) != 0 {
		t.Fatalf("held root lost its policy: %#v %#v", p, c)
	}
	if want := []string{"pin", "policy", "inspect", "close"}; !reflect.DeepEqual(sources.Events, want) || sources.Pinned {
		t.Fatalf("events %v, still pinned %t", sources.Events, sources.Pinned)
	}

	refused := policy.Result{Issues: []policy.Issue{{Kind: policy.Syntax}}}
	sources = &porttest.Credentials{}
	if p, c := domain.InspectCredentials(&porttest.Policies{Result: refused, Root: sources}, sources); !reflect.DeepEqual(p, refused) || c != nil {
		t.Fatalf("policy refusal changed: %#v %#v", p, c)
	}
	if want := []string{"pin", "policy", "close"}; !reflect.DeepEqual(sources.Events, want) || sources.Pinned {
		t.Fatalf("a policy refusal kept the root: %v, pinned %t", sources.Events, sources.Pinned)
	}
}

func TestCredentialInspectionRefusedRootReadsNoPolicy(t *testing.T) {
	refusal := readiness.CredentialResult{Issues: []readiness.CredentialIssue{{Rule: "checkout.path", Index: -1, Subject: "/checkout"}}}
	sources := &porttest.Credentials{Refusal: refusal}
	policies := &porttest.Policies{Result: policy.Result{Policy: &policy.Policy{Credentials: []policy.Credential{{Kind: "claude-token", Source: "/private/token"}}}}, Root: sources}
	p, c := domain.InspectCredentials(policies, sources)
	if p.Policy != nil || len(p.Issues) != 0 || c == nil || !reflect.DeepEqual(*c, refusal) {
		t.Fatalf("refused root: %#v %#v", p, c)
	}
	if policies.Reads != 0 || len(sources.Calls) != 0 || !reflect.DeepEqual(sources.Events, []string{"pin"}) {
		t.Fatalf("refused root was used: reads %d, calls %v, events %v", policies.Reads, sources.Calls, sources.Events)
	}
}
