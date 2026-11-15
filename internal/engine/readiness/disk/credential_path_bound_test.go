package disk

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// The kernel names an open descriptor in at most 4095 bytes: PATH_MAX counts
// the terminator, and readlink of /proc/self/fd/<fd> fails beyond that. No
// seam is replaced here, so this is the host kernel's own bound: a real file
// whose path has exactly 4095 bytes is mapped, and one byte more is refused.
// The adapter walks one component at a time, as the validator does, so it
// reaches both files; only the descriptor's name tells them apart.
func TestCredentialDescriptorPathBoundIsTheKernels(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	for _, size := range []int{4095, 4096} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			base := t.TempDir()
			path := credentialLongFile(t, base, size)
			f := &CredentialFiles{}
			h, _, status := f.Open(0, base)
			if status != port.CredentialReadOK {
				t.Fatal(status)
			}
			defer f.Close(h)
			var node port.CredentialNode
			for _, name := range strings.Split(strings.TrimPrefix(path, base+"/"), "/") {
				if h, node, status = f.Open(h, name); status != port.CredentialReadOK {
					t.Fatalf("component of %d bytes: %v", len(name), status)
				}
				defer f.Close(h)
			}
			if node.Mode&0170000 != 0100000 || node.Size != int64(len("synthetic")) {
				t.Fatalf("did not reach the file: %#v", node)
			}
			d, ok := f.Descriptor(h)
			if size == 4095 {
				if !ok || d.Path != path || len(d.Path) != 4095 || d.MountID <= 0 {
					t.Fatalf("4095-byte path: ok=%v, %d bytes, mount ID %d", ok, len(d.Path), d.MountID)
				}
				return
			}
			if ok || d != (port.CredentialDescriptor{}) {
				t.Fatalf("4096-byte path: ok=%v, %d bytes, mount ID %d", ok, len(d.Path), d.MountID)
			}
		})
	}
}

// credentialLongFile creates private nested directories below base and a
// regular file whose absolute path has exactly size bytes. A path longer than
// PATH_MAX cannot be named in one call, so it works below an open directory.
func credentialLongFile(t *testing.T, base string, size int) string {
	t.Helper()
	parent, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = parent.Close() }()
	path := base
	const leaf = "token"
	// Each nested directory spends one separator and its name.
	for fill := size - len(path) - 1 - len(leaf); fill != 0; {
		name := min(fill-1, 200)
		if fill-1-name == 1 {
			name-- // never leave a single byte, which no separator and name can spend
		}
		if name < 1 {
			t.Fatalf("cannot build a path of %d bytes below %q", size, base)
		}
		component := strings.Repeat("d", name)
		if err := parent.Mkdir(component, 0o700); err != nil {
			t.Fatal(err)
		}
		next, err := parent.OpenRoot(component)
		if err != nil {
			t.Fatal(err)
		}
		_ = parent.Close()
		parent, path, fill = next, path+"/"+component, fill-name-1
	}
	if err := parent.WriteFile(leaf, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if path += "/" + leaf; len(path) != size {
		t.Fatalf("built a path of %d bytes, want %d", len(path), size)
	}
	return path
}
