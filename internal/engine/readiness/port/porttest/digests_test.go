package porttest

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestDigestContract(t *testing.T) {
	DigestContract(t, func(values map[string]string) port.Digests { return Digests(values) })
}
