// Package domain coordinates read-only requirement inspection.
package domain

import (
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// Inspect validates policy without inspecting its requested manifest.
func Inspect(policies port.Policies) policy.Result { return policies.Inspect() }

// InspectManifest short-circuits on policy refusal, preserving its typed result.
// Readiness and engine metadata are not dependencies of file validation.
func InspectManifest(policies port.Policies, manifests port.Manifests) (policy.Result, *manifest.Result) {
	result := Inspect(policies)
	if result.Policy == nil || len(result.Issues) != 0 {
		return result, nil
	}
	validated := manifests.Inspect(result.Policy.Manifest)
	return result, &validated
}
