// Package domain coordinates read-only requirement inspection.
package domain

import (
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// Inspect validates policy without inspecting its requested manifest.
func Inspect(policies port.Policies) policy.Result { return policies.Inspect() }

// InspectCredentials pins the effective root before its policy is read and
// holds it until the sources are observed, so both describe one tree. A root
// that cannot be pinned refuses without a policy read. Sources are never
// observed after a policy refusal. File-only scopes do not call this operation.
func InspectCredentials(policies port.Policies, sources port.Credentials) (policy.Result, *readiness.CredentialResult) {
	if refusal := sources.Pin(); len(refusal.Issues) != 0 {
		return policy.Result{}, &refusal
	}
	defer sources.Close()
	p := Inspect(policies)
	if p.Policy == nil || len(p.Issues) != 0 {
		return p, nil
	}
	r := sources.Inspect(p.Policy.Credentials)
	return p, &r
}

// InspectReadiness requests read-only observations, never approval.
func InspectReadiness(inspector port.Readiness) readiness.Result { return inspector.Inspect() }

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
