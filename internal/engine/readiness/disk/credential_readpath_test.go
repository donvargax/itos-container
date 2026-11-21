package disk

import (
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"os"
	"strings"
	"testing"
)

func TestCredentialReadPathRetainsOnlyOpenDescriptor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "source")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	fs := &CredentialFiles{handles: map[port.CredentialHandle]*credentialFileHandle{1: {file: file}}}
	path, ok := fs.ReadPath(1)
	if !ok || !strings.HasPrefix(path, "/proc/self/fd/") || path == file.Name() {
		t.Fatal("read capability did not name the held descriptor")
	}
	fs.Close(1)
	if path, ok := fs.ReadPath(1); ok || path != "" {
		t.Fatal("closed source retained a readable descriptor")
	}
}
