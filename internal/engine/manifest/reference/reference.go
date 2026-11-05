// Package reference validates image reference syntax without normalization or I/O.
package reference

import (
	_ "crypto/sha256" // Register the only digest algorithm this profile accepts.

	"github.com/distribution/reference"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

// References implements the manifest-owned pinned-reference contract.
type References struct{}

var _ port.References = References{}

// Valid preserves the caller's literal reference and requires both name and digest.
func (References) Valid(image string) bool {
	ref, err := reference.Parse(image)
	if err != nil {
		return false
	}
	// Canonical requires both Named and Digested. Parse already checks the
	// digest's registered algorithm, encoded length and lowercase hex alphabet.
	digested, ok := ref.(reference.Canonical)
	if !ok {
		return false
	}
	return digested.Digest().Algorithm() == "sha256"
}
