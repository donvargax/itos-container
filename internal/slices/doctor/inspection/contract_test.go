package inspection_test

import (
	"testing"

	policytest "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestPoliciesContract(t *testing.T) {
	porttest.PoliciesContract(t, func(f porttest.Fixture) port.Policies {
		return inspection.Policies{Source: &policytest.Source{Value: f.Value, Issues: f.Issues}}
	})
}
