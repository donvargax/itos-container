package disk

import (
	"github.com/donvargax/itos-container/internal/slices/count/port"
	"github.com/donvargax/itos-container/internal/slices/count/port/porttest"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesContract(t *testing.T) {
	porttest.FilesContract(t, func(t *testing.T) (port.Files, porttest.Fixture) {
		dir := t.TempDir()
		paths := porttest.Fixture{Empty: filepath.Join(dir, "empty"), Data: filepath.Join(dir, "data"), Missing: filepath.Join(dir, "missing"), Unreadable: dir}
		for path, data := range map[string][]byte{paths.Empty: {}, paths.Data: []byte("one\x00two\n")} {
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		return Files{}, paths
	})
}
