package domain

import (
	"reflect"
	"testing"

	manifestfake "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
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

// A valid version 2 file is refused as an unsupported version before the manifest,
// the checkout identity or the engine, whatever network it asks for. This is
// run's zero-value gate until its proxy topology can honour that request.
func TestValidVersion2PolicyIsRefusedBeforeAnyLaterStage(t *testing.T) {
	for name, network := range map[string]map[string]policy.Value{
		"allowlist": allow("api.example.com"), "offline": {"mode": {Kind: policy.String, Text: "offline"}}, "default": nil,
	} {
		t.Run(name, func(t *testing.T) {
			i, input, e, c := fixture()
			i.Policy = &policyfake.Source{Value: version2(network)}
			i.Names = &policyfake.Names{Table: map[string]string{"api.example.com": "api.example.com"}}
			files := i.Files.(*manifestfake.Files)
			result := i.Inspect(input)
			want := []policy.Issue{{Kind: policy.Version, Path: []string{"version"}, Version: 2}}
			if result.Ready || result.Policy == nil || result.Policy.Policy != nil || !reflect.DeepEqual(result.Policy.Issues, want) {
				t.Fatalf("policy result %#v", result.Policy)
			}
			if result.Manifest != nil || result.Checkout != nil || len(result.Issues) != 0 || e.Opens != 0 || c.Opened || len(files.Reads) != 0 {
				t.Fatalf("a later stage ran: %#v, opens %d, reads %v", result, e.Opens, files.Reads)
			}
		})
	}
}

func TestVersion1PolicyIsStillAccepted(t *testing.T) {
	i, input, _, _ := fixture()
	i.Names = &policyfake.Names{}
	if result := i.Inspect(input); !result.Ready || result.Policy.Policy == nil || result.Policy.Policy.Version != 1 {
		t.Fatalf("version 1 result %#v", result)
	}
}

// An invalid version 2 file is refused for its own problems, as any invalid policy is.
func TestInvalidVersion2PolicyReportsItsOwnProblem(t *testing.T) {
	i, input, e, _ := fixture()
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
			i, input, e, _ := fixture()
			i.AllowVersion2 = true
			i.Policy = &policyfake.Source{Value: version2(network)}
			i.Names = &policyfake.Names{Table: map[string]string{"api.example.com": "api.example.com"}}
			result := i.Inspect(input)
			if !result.Ready || result.Policy.Policy.Version != 2 || result.Manifest == nil || result.Engine == nil || result.Image == nil || e.Opens != 1 {
				t.Fatal(result, e.Opens)
			}
		})
	}
}
