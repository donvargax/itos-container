package disk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func TestPinnedRootReadsAndErrors(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "itos-container.yaml"), []byte("pinned"), 0600); err != nil {
		t.Fatal(err)
	}
	pin, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pin.Close() }()
	files := Files{OpenRoot: func() (*os.Root, error) { return pin.OpenRoot(".") }}
	if data, failure := files.Read(); string(data) != "pinned" || failure != port.None {
		t.Fatal(string(data), failure)
	}
	if err := pin.Close(); err != nil {
		t.Fatal(err)
	}
	if data, failure := files.Read(); data != nil || failure != port.Unreadable {
		t.Fatal(data, failure)
	}
}
