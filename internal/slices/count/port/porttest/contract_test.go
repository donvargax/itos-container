package porttest

import (
	"github.com/donvargax/itos-container/internal/slices/count/port"
	"testing"
)

func TestFilesContract(t *testing.T) {
	FilesContract(t, func(*testing.T) (port.Files, Fixture) { return ContractFiles() })
}
