package disk_test

import (
	"os"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/disk"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func TestFilesContract(t *testing.T) {
	porttest.FilesContract(t, func(data []byte, failure port.Failure) port.Files {
		t.Chdir(t.TempDir())
		switch failure {
		case port.None:
			if err := os.WriteFile("itos-container.yaml", data, 0600); err != nil {
				t.Fatal(err)
			}
		case port.Unreadable:
			if err := os.Mkdir("itos-container.yaml", 0700); err != nil {
				t.Fatal(err)
			}
		}
		return disk.Files{}
	})
}
