package domain

import (
	"reflect"
	"testing"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	policyfake "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func version2(network map[string]policy.Value) policy.Value {
	fields := map[string]policy.Value{
		"version":  {Kind: policy.Number, Number: policy.Decimal{Coefficient: "2", Exponent: "0"}},
		"manifest": {Kind: policy.String, Text: "worker.json"},
	}
	if network != nil {
		fields["network"] = policy.Value{Kind: policy.Object, Fields: network}
	}
	return policy.Value{Kind: policy.Object, Fields: fields}
}

func allow(entries ...string) map[string]policy.Value {
	items := make([]policy.Value, len(entries))
	for i, entry := range entries {
		items[i] = policy.Value{Kind: policy.String, Text: entry}
	}
	return map[string]policy.Value{"mode": {Kind: policy.String, Text: "allowlist"}, "allow": {Kind: policy.Array, Items: items}}
}

func TestVersion1PolicyIsStillAccepted(t *testing.T) {
	i, input, _, _ := fixture()
	i.Names = &policyfake.Names{}
	if result := i.Inspect(input); !result.Ready || result.Policy.Policy == nil || result.Policy.Policy.Version != 1 {
		t.Fatalf("version 1 result %#v", result)
	}
}

func TestVersion3StillStopsBeforeManifestAndEngine(t *testing.T) {
	i, input, engine, _ := fixture()
	value := version2(nil)
	value.Fields["version"] = policy.Value{Kind: policy.Number, Number: policy.Decimal{Coefficient: "3", Exponent: "0"}}
	i.Policy = &policyfake.Source{Value: value}
	r := i.Inspect(input)
	want := []policy.Issue{{Kind: policy.Version, Path: []string{"version"}, Version: 3}}
	if r.Ready || r.Manifest != nil || r.Checkout != nil || engine.Opens != 0 || r.Policy == nil || r.Policy.Policy != nil || !reflect.DeepEqual(r.Policy.Issues, want) {
		t.Fatalf("version 3 escaped its gate: %#v", r)
	}
}

// An invalid version 2 file is refused for its own problems, as any invalid policy is.
func TestInvalidVersion2PolicyReportsItsOwnProblem(t *testing.T) {
	i, input, e, _ := egressFixture()
	i.Policy = &policyfake.Source{Value: version2(allow("*"))}
	i.Names = &policyfake.Names{}
	result := i.Inspect(input)
	want := []policy.Issue{{Kind: policy.HostName, Path: []string{"network", "allow", "0"}, Version: 2}}
	if result.Ready || result.Policy.Policy != nil || !reflect.DeepEqual(result.Policy.Issues, want) || result.Manifest != nil || e.Opens != 0 {
		t.Fatalf("result %#v", result)
	}
}

func TestReviewInspectionAdmitsVersion2WithoutSkippingPrerequisites(t *testing.T) {
	for name, network := range map[string]map[string]policy.Value{"allowlist": allow("api.example.com"), "offline": nil} {
		t.Run(name, func(t *testing.T) {
			i, input, e, _ := egressFixture()
			i.Policy = &policyfake.Source{Value: version2(network)}
			i.Names = &policyfake.Names{Table: map[string]string{"api.example.com": "api.example.com"}}
			result := i.Inspect(input)
			if !result.Ready || result.Policy.Policy.Version != 2 || result.Manifest == nil || result.Engine == nil || result.Image == nil || e.Opens != 1 {
				t.Fatal(result, e.Opens)
			}
		})
	}
}
