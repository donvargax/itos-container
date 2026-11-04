package domain_test

import (
	"reflect"
	"testing"

	"github.com/donvargax/itos-container/internal/doctor/domain"
	policy "github.com/donvargax/itos-container/internal/policy/port"
)

type policies struct {
	result policy.Result
	reads  int
}

func (p *policies) Inspect() policy.Result { p.reads++; return p.result }

func TestInspectionPreservesPolicyOutcome(t *testing.T) {
	for _, result := range []policy.Result{
		{Policy: &policy.Policy{Version: 1, Manifest: "a.json", Network: policy.Network{Mode: "offline"}}},
		{Issues: []policy.Issue{{Kind: policy.Missing}}},
	} {
		p := &policies{result: result}
		if got := domain.Inspect(p); !reflect.DeepEqual(got, result) || p.reads != 1 {
			t.Fatalf("got %#v after %d reads", got, p.reads)
		}
	}
}
