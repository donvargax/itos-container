package porttest

import (
	"reflect"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// Reporter is the test output used by the inspection contract.
type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// Fixture describes a policy source and its expected inspection outcome.
type Fixture struct {
	Value  policy.Value
	Issues []policy.Issue
	Result policy.Result
	Names  map[string]string // canonical answers for the host names the value lists
}

// PoliciesContract runs against the fake and the policy-engine adapter.
func PoliciesContract(t Reporter, open func(Fixture) port.Policies) {
	t.Helper()
	fixtures := []Fixture{
		{Value: policy.Value{Kind: policy.Object, Fields: map[string]policy.Value{
			"version": {Kind: policy.Number, Number: policy.Decimal{Coefficient: "1", Exponent: "0"}}, "manifest": {Kind: policy.String, Text: "a.json"},
		}}, Result: policy.Result{Policy: &policy.Policy{Version: 1, Manifest: "a.json", Network: policy.Network{Mode: "offline"}}}},
		{Value: policy.Value{Kind: policy.Object, Fields: map[string]policy.Value{
			"version": {Kind: policy.Number, Number: policy.Decimal{Coefficient: "2", Exponent: "0"}}, "manifest": {Kind: policy.String, Text: "a.json"},
			"network": {Kind: policy.Object, Fields: map[string]policy.Value{
				"mode":  {Kind: policy.String, Text: "allowlist"},
				"allow": {Kind: policy.Array, Items: []policy.Value{{Kind: policy.String, Text: "B.example."}, {Kind: policy.String, Text: "a.example"}}},
			}},
		}}, Names: map[string]string{"B.example.": "b.example.", "a.example": "a.example"},
			Result: policy.Result{Policy: &policy.Policy{Version: 2, Manifest: "a.json", Network: policy.Network{Mode: "allowlist", Allow: []string{"a.example", "b.example"}}}}},
		{Issues: []policy.Issue{{Kind: policy.Missing}}, Result: policy.Result{Issues: []policy.Issue{{Kind: policy.Missing}}}},
		{Issues: []policy.Issue{{Kind: policy.Unreadable}}, Result: policy.Result{Issues: []policy.Issue{{Kind: policy.Unreadable}}}},
		{Value: policy.Value{Kind: policy.Object, Fields: map[string]policy.Value{
			"version": {Kind: policy.Number, Number: policy.Decimal{Coefficient: "4", Exponent: "0"}},
		}}, Result: policy.Result{Issues: []policy.Issue{{Kind: policy.Version, Path: []string{"version"}}}}},
	}
	for _, fixture := range fixtures {
		policies := open(fixture)
		for range 2 {
			if got := policies.Inspect(); !reflect.DeepEqual(got, fixture.Result) {
				t.Errorf("Inspect() = %#v; want %#v", got, fixture.Result)
			}
		}
	}
}
