package reference_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/readiness/reference"
)

func TestDigestContract(t *testing.T) {
	porttest.DigestContract(t, func(map[string]string) port.Digests { return reference.Digests{} })
}
