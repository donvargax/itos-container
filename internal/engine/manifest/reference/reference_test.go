package reference_test

import (
	_ "crypto/sha512" // A registered non-sha256 algorithm is still refused.
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/manifest/reference"
	"testing"
)

func TestReferenceContract(t *testing.T) {
	porttest.ReferenceContract(t, func(string, bool) port.References { return reference.References{} })
}
