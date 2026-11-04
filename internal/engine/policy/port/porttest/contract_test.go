package porttest_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func TestFilesContract(t *testing.T) {
	porttest.FilesContract(t, func(data []byte, failure port.Failure) port.Files {
		return porttest.Files{Data: data, Failure: failure}
	})
}

func TestSourceContract(t *testing.T) {
	porttest.SourceContract(t, func(f porttest.SourceFixture) port.Source {
		return &porttest.Source{Value: f.Value, Issues: f.Issues}
	})
}
