package idna_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/idna"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func TestNamesContract(t *testing.T) {
	porttest.NamesContract(t, func([]porttest.NameFixture) port.Names { return idna.Names{} })
}
