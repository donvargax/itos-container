package yaml_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	policyyaml "github.com/donvargax/itos-container/internal/engine/policy/yaml"
)

func TestSourceContract(t *testing.T) {
	porttest.SourceContract(t, func(f porttest.SourceFixture) port.Source {
		return policyyaml.Source{Files: porttest.Files{Data: []byte(f.Data), Failure: f.Failure}}
	})
}
