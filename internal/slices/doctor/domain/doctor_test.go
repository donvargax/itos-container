package domain_test

import (
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"reflect"
	"testing"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/domain"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestInspectionPreservesPolicyOutcome(t *testing.T) {
	for _, result := range []policy.Result{
		{Policy: &policy.Policy{Version: 1, Manifest: "a.json", Network: policy.Network{Mode: "offline"}}},
		{Issues: []policy.Issue{{Kind: policy.Missing}}},
	} {
		p := &porttest.Policies{Result: result}
		if got := domain.Inspect(p); !reflect.DeepEqual(got, result) || p.Reads != 1 {
			t.Fatalf("got %#v after %d reads", got, p.Reads)
		}
	}
}

func TestPolicyRefusalSkipsManifest(t *testing.T) {
	for _, result := range []policy.Result{{Issues: []policy.Issue{{Kind: policy.Missing}}}, {Policy: &policy.Policy{Manifest: "selected.json"}, Issues: []policy.Issue{{Kind: policy.Syntax}}}} {
		policies := &porttest.Policies{Result: result}
		manifests := &porttest.Manifests{}
		got, checked := domain.InspectManifest(policies, manifests)
		if !reflect.DeepEqual(got, result) || checked != nil || len(manifests.Paths) != 0 || policies.Reads != 1 {
			t.Fatalf("policy short-circuit: %#v, %#v, %v", got, checked, manifests.Paths)
		}
	}
}

func TestManifestUsesOnlyTheSelectedPath(t *testing.T) {
	policies := &porttest.Policies{Result: policy.Result{Policy: &policy.Policy{Manifest: "config files/selected.json"}}}
	want := manifest.Result{Path: "config files/selected.json", Issues: []manifest.Issue{{Kind: manifest.Syntax}}}
	manifests := &porttest.Manifests{Result: want}
	_, got := domain.InspectManifest(policies, manifests)
	if got == nil || !reflect.DeepEqual(*got, want) || !reflect.DeepEqual(manifests.Paths, []string{"config files/selected.json"}) {
		t.Fatalf("manifest request = %#v, %v", got, manifests.Paths)
	}
}
