package domain_test

import (
	"reflect"
	"testing"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
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
