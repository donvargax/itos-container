package input_test

import (
	"github.com/donvargax/itos-container/internal/slices/count/input"
	"github.com/donvargax/itos-container/internal/slices/count/port"
	"github.com/donvargax/itos-container/internal/slices/count/port/porttest"
	"strings"
	"testing"
)

func TestFilesContract(t *testing.T) {
	porttest.FilesContract(t, func(*testing.T) (port.Files, porttest.Fixture) {
		files, paths := porttest.ContractFiles()
		return input.Files{Disk: files, In: strings.NewReader("must not be read")}, paths
	})
}
