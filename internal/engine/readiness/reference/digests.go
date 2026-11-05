// Package reference compares repository digests without registry or engine I/O.
package reference

import (
	_ "crypto/sha256" // Register the digest algorithm accepted by the profile.

	"github.com/distribution/reference"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Digests uses the same pinned reference grammar as manifest validation.
type Digests struct{}

var _ port.Digests = Digests{}

// Canonical normalizes the repository and keeps only its sha256 digest.
func (Digests) Canonical(value string) string {
	named, err := reference.ParseNormalizedNamed(value)
	if err != nil {
		return ""
	}
	digested, ok := named.(reference.Canonical)
	if !ok || digested.Digest().Algorithm() != "sha256" {
		return ""
	}
	return digested.Name() + "@" + digested.Digest().String()
}
