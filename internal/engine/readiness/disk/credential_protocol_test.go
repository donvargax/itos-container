package disk

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestCredentialStatxMountEvidence(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	f := &CredentialFiles{}
	h, _, status := f.Open(0, t.TempDir())
	if status != port.CredentialReadOK {
		t.Fatal(status)
	}
	defer f.Close(h)
	d, ok := f.Descriptor(h)
	if !ok || d.MountID == 0 {
		t.Fatalf("native statx mount identity unavailable: %#v %v", d, ok)
	}
	for _, tc := range []struct {
		name     string
		id       uint64
		mask     uint32
		err      error
		want     port.CredentialDescriptor
		complete bool
	}{
		{"exact field", uint64(d.MountID), 0x1000, nil, d, true},
		{"additional fields", uint64(d.MountID), 0x1001, nil, d, true},
		{"unsupported call", 0, 0, errors.ErrUnsupported, port.CredentialDescriptor{}, true},
		{"field absent", uint64(d.MountID), 0, nil, port.CredentialDescriptor{}, true},
		{"unique ID is not mountinfo ID", uint64(d.MountID), 0x4000, nil, port.CredentialDescriptor{}, true},
		{"zero ID", 0, 0x1000, nil, port.CredentialDescriptor{}, true},
		{"overflow ID", 1 << 32, 0x1000, nil, port.CredentialDescriptor{}, true},
		{"fdinfo disagrees", uint64(d.MountID) + 1, 0x1000, nil, port.CredentialDescriptor{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ops := f.operations()
			ops.statx = func(fd int) (uint64, uint32, error) {
				if fd != int(f.handles[h].file.Fd()) {
					t.Fatal("statx did not use the held descriptor")
				}
				return tc.id, tc.mask, tc.err
			}
			f.ops = &ops
			defer func() { f.ops = nil }()
			got, complete := f.Descriptor(h)
			if got != tc.want || complete != tc.complete {
				t.Fatalf("descriptor=%#v complete=%v, want %#v %v", got, complete, tc.want, tc.complete)
			}
		})
	}
}

// mountinfo's ID field has 32 bits. The largest one is evidence when fdinfo
// agrees; only a wider statx value is refused.
func TestCredentialLargestMountID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	f := &CredentialFiles{}
	root := t.TempDir()
	h, _, status := f.Open(0, root)
	if status != port.CredentialReadOK {
		t.Fatal(status)
	}
	defer f.Close(h)
	ops := f.operations()
	ops.statx = func(int) (uint64, uint32, error) { return 1<<32 - 1, 0x1000, nil }
	ops.read = func(string, int64) ([]byte, bool) { return []byte("mnt_id:\t4294967295\n"), true }
	f.ops = &ops
	want := port.CredentialDescriptor{Path: root, MountID: 1<<32 - 1}
	if got, ok := f.Descriptor(h); !ok || got != want {
		t.Fatalf("descriptor=%#v complete=%v, want %#v", got, ok, want)
	}
}

func TestCredentialMountEvidenceValidation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("kernel-format parser is Linux-specific")
	}
	valid := "1 0 0:1 / / rw unknown:field - tmpfs tmpfs rw\n"
	for _, data := range []string{"", strings.TrimSuffix(valid, "\n"), "broken\n", strings.Replace(valid, "1 0", "bad 0", 1), strings.Replace(valid, "1 0", "1 -1", 1), strings.Replace(valid, "0:1", "0:1:2", 1), strings.Replace(valid, "0:1", "x:1", 1), strings.Replace(valid, "1 0", "4294967296 0", 1), strings.Replace(valid, "1 0", "0 0", 1), strings.Replace(valid, " / / ", " / /a/../b ", 1), strings.Replace(valid, " / / ", " / //a ", 1), strings.Replace(valid, " / / ", " / /a\\000 ", 1), strings.Replace(valid, " / / ", " /a\\000 / ", 1), strings.Replace(valid, " / / ", " / /a\\xxx ", 1), strings.Replace(valid, " - ", "  ", 1), valid + valid} {
		if rows, ok := ParseCredentialMounts([]byte(data)); ok || rows != nil {
			t.Fatalf("accepted malformed topology %q: %#v", data, rows)
		}
	}
	data := "1 0 8:257 /root\\040space /mount\\040space rw unknown:ok - tmpfs tmpfs rw\n"
	rows, ok := ParseCredentialMounts([]byte(data))
	if !ok || len(rows) != 1 || rows[0].Root != "/root space" || rows[0].Point != "/mount space" || rows[0].Device != 1050625 {
		t.Fatalf("escaped coordinates: %#v %v", rows, ok)
	}
	// A row's root and its mountpoint each keep the parser's own 4096 bytes:
	// mountinfo is not held to PATH_MAX, unlike a descriptor's pathname.
	for _, size := range []int{4096, 4097} {
		path := "/" + strings.Repeat("a", size-1)
		_, ok := ParseCredentialMounts([]byte("1 0 0:1 " + path + " / rw - tmpfs tmpfs rw\n"))
		if ok != (size == 4096) {
			t.Fatalf("mount path length %d accepted=%v", size, ok)
		}
		rows, ok := ParseCredentialMounts([]byte("1 0 0:1 / " + path + " rw - tmpfs tmpfs rw\n"))
		if ok != (size == 4096) || ok && (len(rows) != 1 || rows[0].Point != path || rows[0].Unprojectable) {
			t.Fatalf("mountpoint length %d accepted=%v: %d rows", size, ok, len(rows))
		}
	}
	// makedev(3): the low 8 minor bits, 12 major bits from bit 8, the remaining
	// minor bits from bit 20 and the remaining major bits from bit 32.
	rows, ok = ParseCredentialMounts([]byte("1 0 4097:258 / / rw - tmpfs tmpfs rw\n"))
	if want := uint64(2 | 1<<8 | 1<<20 | 1<<44); !ok || len(rows) != 1 || rows[0].Device != want {
		t.Fatalf("wide device number: %#v %v, want %d", rows, ok, want)
	}
}

func TestCredentialEmptyMountSource(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("kernel-format parser is Linux-specific")
	}
	for _, optional := range []string{"", "shared:7 master:1 "} {
		valid := "473 411 0:59 / /scratch rw,relatime " + optional + "- tmpfs tmpfs rw,uid=1000"
		fields := strings.Split(valid, " ")
		for i := range fields {
			t.Run(fmt.Sprintf("optional=%q/empty=%d", optional, i), func(t *testing.T) {
				row := append([]string(nil), fields...)
				row[i] = ""
				rows, ok := ParseCredentialMounts([]byte(strings.Join(row, " ") + "\n"))
				if i != len(fields)-2 {
					if ok || rows != nil {
						t.Fatalf("accepted empty non-source field %d: %#v", i, rows)
					}
					return
				}
				want := port.CredentialMount{ID: 473, Device: 59, Root: "/", Point: "/scratch", Filesystem: "tmpfs"}
				if !ok || len(rows) != 1 || rows[0] != want {
					t.Fatalf("empty source lost mount coordinates: %#v %v", rows, ok)
				}
			})
		}
	}
}

func TestCredentialFDInfoBounds(t *testing.T) {
	// A refusal carries no ID: 1 is a real mount ID, and a caller reading the
	// number beside a false result must not find a row to match.
	for _, data := range []string{"", "pos:\t0\n", "mnt_id\n", "mnt_id:\n", "mnt_id: 0\n", "mnt_id: -1\n", "mnt_id: +1\n", "mnt_id: nope\n", "mnt_id: 99999999999999999999999\n", "mnt_id: 1\nmnt_id: 2\n", "mnt_id: 7\nmnt_id: 7\n", "mnt_id: 7\nmnt_id: nope\n", "mnt_id: 0\nmnt_id: 5\n"} {
		if id, ok := ParseCredentialMountID([]byte(data)); ok || id != 0 {
			t.Fatalf("refused fdinfo %q returned %d %v", data, id, ok)
		}
	}
	// The kernel's other fdinfo lines carry numbers too; only mnt_id is the ID.
	for data, want := range map[string]int{"mnt_id:\t1\n": 1, "pos:\t0\nflags:\t02000000\nmnt_id:\t42\nino:\t7\n": 42} {
		if id, ok := ParseCredentialMountID([]byte(data)); !ok || id != want {
			t.Fatalf("fdinfo %q returned %d %v, want %d", data, id, ok, want)
		}
	}
	for _, size := range []int{65536, 65537} {
		data := "mnt_id:\t42\n" + strings.Repeat("x", size-len("mnt_id:\t42\n"))
		id, ok := ParseCredentialMountID([]byte(data))
		want := 42
		if size > 65536 {
			want = 0
		}
		if ok != (size == 65536) || id != want {
			t.Fatalf("fdinfo size %d: %d %v", size, id, ok)
		}
		if runtime.GOOS != "linux" {
			continue
		} // Native /proc paths are Linux-only; parser cases above remain portable.
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

func TestCredentialNonPathMountRoots(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("kernel-format parser is Linux-specific")
	}
	for _, root := range []string{"net:[4026537384]", "mnt:[4026537385]", "/replaced//deleted", "relative"} {
		data := "1 0 0:1 / / rw - tmpfs tmpfs rw\n2 1 0:4 " + root + " /run/netns/test rw - nsfs nsfs rw\n"
		rows, ok := ParseCredentialMounts([]byte(data))
		if !ok || len(rows) != 2 || rows[0].Unprojectable || !rows[1].Unprojectable || rows[1].Root != root || rows[1].Point != "/run/netns/test" || rows[1].ID != 2 {
			t.Fatalf("lost unprojectable mount row for %q: %#v %v", root, rows, ok)
		}
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
	credentialEntriesRefused(t, f, h, "regular source used as a directory")
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
	// The source's pin is still held here, so a refusal that handed out any
	// nonzero number could name it.
	credentialOpenRefused(t, f, 999, "child", port.CredentialReadUnavailable)
	credentialOpenRefused(t, f, 0, root+"/missing", port.CredentialReadMissing)
	if _, ok := f.Stat(999); ok {
		t.Fatal("unknown handle stat accepted")
	}
	if _, ok := f.Descriptor(999); ok {
		t.Fatal("unknown descriptor accepted")
	}
	credentialEntriesRefused(t, f, 999, "unknown directory accepted")
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
				credentialOpenRefused(t, f, 0, path, port.CredentialReadUnavailable)
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
	credentialOpenRefused(t, f, 0, dir, port.CredentialReadUnavailable)
}

// A source is never opened for reading. Listing a pin that is not a directory
// is refused from its metadata, before anything is reopened.
func TestCredentialEntriesNeverReopenAFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	path := t.TempDir() + "/source"
	if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &CredentialFiles{}
	h, _, status := f.Open(0, path)
	if status != port.CredentialReadOK {
		t.Fatal(status)
	}
	defer f.Close(h)
	ops := f.operations()
	ops.directory = func(name string) (*os.File, error) {
		t.Error("reopened a regular file for reading")
		return os.Open(name)
	}
	f.ops = &ops
	credentialEntriesRefused(t, f, h, "regular source listed as a directory")
}

// Close releases the pin and the directory reopened to list it, and forgets
// the handle, so a long inventory holds descriptors only for its current depth.
func TestCredentialCloseReleasesBothDescriptors(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	f := &CredentialFiles{}
	h, _, status := f.Open(0, t.TempDir())
	if status != port.CredentialReadOK {
		t.Fatal(status)
	}
	if _, done, ok := f.Entries(h); !ok || !done {
		t.Fatalf("empty directory: done=%v ok=%v", done, ok)
	}
	held := f.handles[h]
	if held == nil || held.directory == nil {
		t.Fatal("listing kept no reopened directory")
	}
	f.Close(h)
	if _, err := held.file.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("pin still open after Close: %v", err)
	}
	if _, err := held.directory.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("reopened directory still open after Close: %v", err)
	}
	if len(f.handles) != 0 {
		t.Fatalf("Close kept %d handles", len(f.handles))
	}
}

// A refused open hands out no handle and no metadata. Handles are small
// counters, so a nonzero one beside a failure status could name a live pin, and
// a caller closing what it was given would release another object's descriptor.
func credentialOpenRefused(t *testing.T, f *CredentialFiles, parent port.CredentialHandle, name string, want port.CredentialRead) {
	t.Helper()
	held, descriptors := len(f.handles), credentialOpenDescriptors(t)
	h, node, status := f.Open(parent, name)
	if h != 0 || node != (port.CredentialNode{}) || status != want {
		t.Fatalf("open %q under %d returned handle %d, node %#v and status %v; want no handle, no metadata and status %v", name, parent, h, node, status, want)
	}
	if len(f.handles) != held {
		t.Fatalf("refused open %q under %d retained a pin: %d held, was %d", name, parent, len(f.handles), held)
	}
	// A descriptor opened before the refusal is closed with it, not left to a
	// finalizer: an inventory refuses many entries within one descriptor limit.
	if after := credentialOpenDescriptors(t); after != descriptors {
		t.Fatalf("refused open %q under %d left %d descriptors open, was %d", name, parent, after, descriptors)
	}
}

func credentialOpenDescriptors(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// A failed directory read reports neither names nor completion. Completion
// beside a failure would read as a fully enumerated directory, and names would
// be a prefix of one.
func credentialEntriesRefused(t *testing.T, f *CredentialFiles, h port.CredentialHandle, what string) {
	t.Helper()
	if names, done, ok := f.Entries(h); names != nil || done || ok {
		t.Fatalf("%s: names=%q done=%v ok=%v", what, names, done, ok)
	}
}

func TestCredentialUnusableNativeHandles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native descriptor calls require Linux")
	}
	for _, mode := range []string{"closed pin", "directory unavailable", "directory replacement", "directory read failure", "invalid fdinfo"} {
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
			case "directory read failure":
				// The kernel can fail a batch after yielding part of it.
				ops.entries = func(*os.File, int) ([]os.DirEntry, error) {
					return []os.DirEntry{credentialEntryName("partial")}, os.ErrPermission
				}
			case "invalid fdinfo":
				ops.read = func(string, int64) ([]byte, bool) { return []byte("mnt_id: bad"), true }
			}
			f.ops = &ops
			if mode == "invalid fdinfo" {
				if _, ok := f.Descriptor(h); ok {
					t.Fatal("invalid descriptor accepted")
				}
			} else {
				credentialEntriesRefused(t, f, h, "unusable directory accepted")
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
	// The largest 32-bit owner and mode are metadata, and each field keeps its place.
	tree := fstest.MapFS{"file": {Data: []byte("synthetic"), Sys: struct{ Dev, Ino, Nlink, Uid, Mode uint64 }{1, 2, 3, 1<<32 - 1, 1<<32 - 1}}}
	info, err := tree.Stat("file")
	if err != nil {
		t.Fatal(err)
	}
	want := port.CredentialNode{Device: 1, Inode: 2, Links: 3, UID: 1<<32 - 1, Mode: 1<<32 - 1, Size: int64(len("synthetic"))}
	if got, ok := credentialMetadata(info); !ok || got != want {
		t.Fatalf("metadata=%#v complete=%v, want %#v", got, ok, want)
	}
}

// The byte bound is independent of row count, row length and ID uniqueness.
// Exercise both the parser and the production bounded reader used by Mounts.
func TestCredentialMountTableByteBound(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native mount observations require Linux")
	}
	var table strings.Builder
	for i := 1; i <= 129; i++ {
		point := "/"
		if i > 1 {
			point = fmt.Sprintf("/unrelated/%d", i)
		}
		line := fmt.Sprintf("%d 0 0:1 / %s rw x:", i, point)
		end := " - tmpfs tmpfs rw"
		table.WriteString(line + strings.Repeat("a", 65535-len(line)-len(end)) + end + "\n")
		if i < 128 {
			continue
		}
		data := []byte(table.String())
		want := i == 128
		if rows, ok := ParseCredentialMounts(data); ok != want || ok && len(rows) != i {
			t.Fatalf("parser rows=%d: complete=%v count=%d", i, ok, len(rows))
		}
		path := t.TempDir() + "/mountinfo"
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		files := &CredentialFiles{}
		ops := files.operations()
		ops.read = func(_ string, limit int64) ([]byte, bool) {
			read, complete := credentialReadBounded(path, limit)
			if complete != want || len(read) != min(len(data), 8*1024*1024+1) {
				t.Fatalf("reader rows=%d: complete=%v bytes=%d", i, complete, len(read))
			}
			return read, complete
		}
		files.ops = &ops
		if _, ok := files.Mounts(); ok != want {
			t.Fatalf("Mounts rows=%d: complete=%v", i, ok)
		}
	}
}
