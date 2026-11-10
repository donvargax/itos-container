package disk

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestCredentialMetadataWalk(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	path := t.TempDir() + "/source"
	if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &CredentialFiles{}
	var parent port.CredentialHandle
	for _, name := range append([]string{"/"}, strings.Split(path[1:], "/")...) {
		h, n, status := f.Open(parent, name)
		if status != port.CredentialReadOK {
			t.Fatalf("open %q under %d: %v", name, parent, status)
		}
		if n.Inode == 0 || n.Links == 0 {
			t.Fatalf("missing identity for %q", name)
		}
		t.Cleanup(func() { f.Close(h) })
		parent = h
	}
}
