package domain_test

import (
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
