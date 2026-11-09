package inspection_test

import (
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	manifesttest "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	"testing"

	policytest "github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestPoliciesContract(t *testing.T) {
	porttest.PoliciesContract(t, func(f porttest.Fixture) port.Policies {
		return inspection.Policies{Source: &policytest.Source{Value: f.Value, Issues: f.Issues}, Names: &policytest.Names{Table: f.Names}}
	})
}

func TestManifestsContract(t *testing.T) {
	porttest.ManifestsContract(t, func(f porttest.ManifestFixture) port.Manifests {
		return inspection.Manifests{
			Files:  &manifesttest.Files{Data: map[string][]byte{"selected.json": []byte("raw")}, Failures: map[string]manifest.Failure{"selected.json": f.Failure}},
			Parser: &manifesttest.Parser{Value: f.Value}, References: &manifesttest.References{Images: map[string]bool{"requested": true}},
		}
	})
}
