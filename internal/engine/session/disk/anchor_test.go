package disk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestDiskAnchorWorkspaceContract(t *testing.T) {
	root := filepath.Join(t.TempDir(), "checkout")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	w := &Workspace{MountStat: func(path string) (fs.FileInfo, error) {
		if path != "/proc/456/root/workspace" {
			t.Errorf("unexpected mount observation path %s", path)
		}
		return info, nil
	}}
	b := port.Binding{Workspace: "/workspace"}
	b.Checkout.Root = root
	if f := w.Pin(b); f != "" {
		t.Fatal(f)
	}
	defer w.Close()
	probe, err := openPath(w.roots[0])
	if errors.Is(err, errors.ErrUnsupported) {
		if _, f := w.Source(); f != port.MountUnavailable {
			t.Fatal(f)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = probe.Close()
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	b.Checkout.Device = strconv.FormatUint(stat.FieldByName("Dev").Uint(), 10)
	b.Checkout.Inode = strconv.FormatUint(stat.FieldByName("Ino").Uint(), 10)
	porttest.AnchorWorkspaceContract(t, w, b, 456, func() {
		if err := os.Rename(root, root+".original"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		source, f := w.Source()
		pinned, err := os.Stat(source.Path)
		if f != "" || err != nil || !os.SameFile(info, pinned) {
			t.Fatal("descriptor lost original inode", f, err)
		}
	})
}

func TestMountedIdentityRequiresReadableDirectoryStat(t *testing.T) {
	for _, fail := range []string{"unavailable", "regular file", "missing kernel identity"} {
		t.Run(fail, func(t *testing.T) {
			w := &Workspace{MountStat: func(string) (fs.FileInfo, error) {
				if fail == "unavailable" {
					return nil, fs.ErrPermission
				}
				path := t.TempDir()
				if fail == "regular file" {
					path = filepath.Join(path, "file")
					if err := os.WriteFile(path, nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
				info, err := os.Stat(path)
				if fail == "missing kernel identity" {
					return noKernelIdentity{info}, err
				}
				return info, err
			}}
			if source, f := w.Mounted(456); f != port.MountRefused || source != (port.MountSource{}) {
				t.Fatal(source, f)
			}
		})
	}
}

type noKernelIdentity struct{ fs.FileInfo }

func (noKernelIdentity) Sys() any { return nil }

func TestSourceRefusesDirectoryWithoutKernelIdentity(t *testing.T) {
	b := port.Binding{Workspace: "/workspace"}
	b.Checkout.Root = t.TempDir()
	var observed *os.File
	w := &Workspace{ops: &workspaceOperations{sourceStat: func(file *os.File) (fs.FileInfo, error) {
		observed = file
		info, err := file.Stat()
		return noKernelIdentity{info}, err
	}}}
	t.Cleanup(w.Close)
	if failure := w.Pin(b); failure != "" {
		t.Fatal(failure)
	}
	// A real open directory makes this stat test independent of O_PATH support.
	source, err := os.Open(b.Checkout.Root)
	if err != nil {
		t.Fatal(err)
	}
	w.source = source
	if got, failure := w.Source(); failure != port.MountUnavailable || got != (port.MountSource{}) {
		t.Fatal("directory without kernel identity supplied authority", got, failure)
	}
	if observed != source {
		t.Fatal("stat did not use the held source descriptor")
	}
	w.Close()
	if _, err := source.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("source descriptor was retained after close", err)
	}
}

func TestMountIdentityRequiresUnsignedDeviceAndInode(t *testing.T) {
	for _, system := range []any{
		nil, 1, (*struct{ Dev, Ino uint64 })(nil),
		struct{ Dev uint64 }{1}, struct{ Ino uint64 }{2},
		struct{ Dev, Ino int64 }{1, 2},
		struct {
			Dev uint64
			Ino string
		}{1, "2"},
	} {
		device, inode, ok := mountIdentity(kernelIdentityInfo{system: system})
		if ok || device != "" || inode != "" {
			t.Fatalf("invalid kernel identity %#v accepted: %s:%s, %v", system, device, inode, ok)
		}
	}
	stat := struct{ Dev, Ino uint64 }{1<<63 + 7, 1<<63 + 9}
	for _, system := range []any{stat, &stat} {
		device, inode, ok := mountIdentity(kernelIdentityInfo{system: system})
		if !ok || device != "9223372036854775815" || inode != "9223372036854775817" {
			t.Fatal("kernel identity was lost or truncated", device, inode, ok)
		}
	}
}

type kernelIdentityInfo struct {
	fs.FileInfo
	system any
}

func (info kernelIdentityInfo) Sys() any { return info.system }
