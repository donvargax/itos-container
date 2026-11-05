package porttest_test

import (
	"testing"

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
