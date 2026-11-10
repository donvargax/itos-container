package disk

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestCredentialMountEvidenceValidation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("kernel-format parser is Linux-specific")
	}
	valid := "1 0 0:1 / / rw unknown:field - tmpfs tmpfs rw\n"
	for _, data := range []string{"", strings.TrimSuffix(valid, "\n"), "broken\n", strings.Replace(valid, "1 0", "bad 0", 1), strings.Replace(valid, "1 0", "1 -1", 1), strings.Replace(valid, "0:1", "0:1:2", 1), strings.Replace(valid, "0:1", "x:1", 1), strings.Replace(valid, "1 0", "4294967296 0", 1), strings.Replace(valid, "1 0", "0 0", 1), strings.Replace(valid, " / / ", " relative / ", 1), strings.Replace(valid, " / / ", " / /a/../b ", 1), strings.Replace(valid, " / / ", " / //a ", 1), strings.Replace(valid, " / / ", " / /a\\000 ", 1), strings.Replace(valid, " / / ", " / /a\\xxx ", 1), strings.Replace(valid, " - ", "  ", 1), valid + valid} {
		if rows, ok := ParseCredentialMounts([]byte(data)); ok || rows != nil {
			t.Fatalf("accepted malformed topology %q: %#v", data, rows)
		}
	}
	data := "1 0 8:257 /root\\040space /mount\\040space rw unknown:ok - tmpfs tmpfs rw\n"
	rows, ok := ParseCredentialMounts([]byte(data))
	if !ok || len(rows) != 1 || rows[0].Root != "/root space" || rows[0].Point != "/mount space" || rows[0].Device != 1050625 {
		t.Fatalf("escaped coordinates: %#v %v", rows, ok)
	}
	for _, size := range []int{4096, 4097} {
		path := "/" + strings.Repeat("a", size-1)
		_, ok := ParseCredentialMounts([]byte("1 0 0:1 " + path + " / rw - tmpfs tmpfs rw\n"))
		if ok != (size == 4096) {
			t.Fatalf("mount path length %d accepted=%v", size, ok)
		}
	}
}

func TestCredentialFDInfoBounds(t *testing.T) {
	for _, data := range []string{"", "mnt_id: 0\n", "mnt_id: -1\n", "mnt_id: +1\n", "mnt_id: nope\n", "mnt_id: 99999999999999999999999\n", "mnt_id: 1\nmnt_id: 2\n"} {
		if _, ok := ParseCredentialMountID([]byte(data)); ok {
			t.Fatalf("accepted %q", data)
		}
	}
	for _, size := range []int{65536, 65537} {
		data := "mnt_id:\t42\n" + strings.Repeat("x", size-len("mnt_id:\t42\n"))
		id, ok := ParseCredentialMountID([]byte(data))
		if ok != (size == 65536) || ok && id != 42 {
			t.Fatalf("fdinfo size %d: %d %v", size, id, ok)
		}
		path := t.TempDir() + "/synthetic-fdinfo"
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		read, complete := credentialReadBounded(path, 65536)
		if complete != (size == 65536) || len(read) != size {
			t.Fatalf("EOF probe %d: %d %v", size, len(read), complete)
		}
	}
	if _, ok := credentialReadBounded(t.TempDir()+"/absent", 1); ok {
		t.Fatal("missing evidence accepted")
	}
}

func TestCredentialNativeErrorsAndPinnedIdentity(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	root := t.TempDir()
	path := root + "/source"
	if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &CredentialFiles{}
	h, before, status := f.Open(0, path)
	if status != port.CredentialReadOK {
		t.Fatal(status)
	}
	defer f.Close(h)
	if _, _, ok := f.Entries(h); ok {
		t.Fatal("regular source used as a directory")
	}
	if err := os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	after, ok := f.Stat(h)
	if !ok || before != after {
		t.Fatal("held identity followed replacement")
	}
	d, ok := f.Descriptor(h)
	if !ok || d.Path != path+"-old" || d.MountID <= 0 {
		t.Fatalf("descriptor mapping lost: %#v %v", d, ok)
	}
	if _, _, status := f.Open(999, "child"); status != port.CredentialReadUnavailable {
		t.Fatal(status)
	}
	if _, _, status := f.Open(0, root+"/missing"); status != port.CredentialReadMissing {
		t.Fatal(status)
	}
	if _, ok := f.Stat(999); ok {
		t.Fatal("unknown handle stat accepted")
	}
	if _, ok := f.Descriptor(999); ok {
		t.Fatal("unknown descriptor accepted")
	}
	if _, _, ok := f.Entries(999); ok {
		t.Fatal("unknown directory accepted")
	}
	f.Close(999)
	for _, fault := range []string{"lstat", "open", "metadata", "read", "readlink", "long readlink"} {
		t.Run(fault, func(t *testing.T) {
			ops := f.operations()
			switch fault {
			case "lstat":
				ops.lstat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
			case "open":
				ops.open = func(int, string) (int, error) { return -1, os.ErrPermission }
			case "metadata":
				ops.metadata = func(*os.File, os.FileInfo) (port.CredentialNode, bool) { return port.CredentialNode{}, false }
			case "read":
				ops.read = func(string, int64) ([]byte, bool) { return nil, false }
			case "readlink":
				ops.readlink = func(string, []byte) (int, error) { return 0, os.ErrPermission }
			case "long readlink":
				ops.readlink = func(_ string, b []byte) (int, error) { return len(b), nil }
			}
			f.ops = &ops
			defer func() { f.ops = nil }()
			if fault == "lstat" || fault == "open" || fault == "metadata" {
				if _, _, status := f.Open(0, path); status != port.CredentialReadUnavailable {
					t.Fatal(status)
				}
			} else if _, ok := f.Descriptor(h); ok {
				t.Fatal("incomplete descriptor accepted")
			}
		})
	}
	// A directory changed after its metadata observation must never become a
	// successfully pinned replacement, even when both versions are safe.
	dir := root + "/directory"
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ops := f.operations()
	ops.open = func(parent int, name string) (int, error) {
		if err := os.Rename(dir, dir+"-old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		return credentialOpen(parent, name)
	}
	f.ops = &ops
	if _, _, status := f.Open(0, dir); status != port.CredentialReadUnavailable {
		t.Fatal("directory substitution accepted")
	}
}

func TestCredentialUnusableNativeHandles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	for _, mode := range []string{"closed pin", "directory unavailable", "directory replacement", "invalid fdinfo"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			f := &CredentialFiles{}
			h, _, status := f.Open(0, root)
			if status != port.CredentialReadOK {
				t.Fatal(status)
			}
			defer f.Close(h)
			ops := f.operations()
			switch mode {
			case "closed pin":
				if err := f.handles[h].file.Close(); err != nil {
					t.Fatal(err)
				}
				if _, ok := f.Stat(h); ok {
					t.Fatal("closed pin accepted")
				}
			case "directory unavailable":
				ops.directory = func(string) (*os.File, error) { return nil, os.ErrPermission }
			case "directory replacement":
				other := t.TempDir()
				ops.directory = func(string) (*os.File, error) { return os.Open(other) }
			case "invalid fdinfo":
				ops.read = func(string, int64) ([]byte, bool) { return []byte("mnt_id: bad"), true }
			}
			f.ops = &ops
			if mode == "invalid fdinfo" {
				if _, ok := f.Descriptor(h); ok {
					t.Fatal("invalid descriptor accepted")
				}
			} else if _, _, ok := f.Entries(h); ok {
				t.Fatal("unusable directory accepted")
			}
		})
	}
}

func TestCredentialMetadataRequiresUnixIdentity(t *testing.T) {
	for _, raw := range []any{nil, 42, struct{ Dev uint64 }{Dev: 1}, struct{ Dev int }{Dev: 1}, struct{ Dev, Ino, Nlink, Uid, Mode uint64 }{Uid: 1 << 32}, struct{ Dev, Ino, Nlink, Uid, Mode uint64 }{Mode: 1 << 32}} {
		tree := fstest.MapFS{"file": {Data: []byte("synthetic"), Sys: raw}}
		info, err := tree.Stat("file")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := credentialMetadata(info); ok {
			t.Fatalf("accepted incomplete identity %#v", raw)
		}
	}
}
