package disk

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestMountSystemErrorOmitsPrivatePayloads(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want port.MountSystemError
	}{
		{nil, port.MountSystemError{}},
		{&os.PathError{Op: "private operation", Path: "private path", Err: syscall.EACCES}, port.MountSystemError{Errno: 13, Message: "permission denied"}},
		{errors.New("private response body"), port.MountSystemError{Message: "unclassified system error"}},
	} {
		if got := mountSystemError(tc.err); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
}

func TestDiskMountErrorContract(t *testing.T) {
	w := &Workspace{}
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.MountErrorContract(t, w, func(fail bool) {
		w.MountStat = func(string) (fs.FileInfo, error) {
			if fail {
				return nil, &os.PathError{Op: "stat", Path: "private path", Err: syscall.EACCES}
			}
			return info, nil
		}
		_, _ = w.Mounted(123)
	})
	file, err := os.CreateTemp(t.TempDir(), "process-reference")
	if err != nil {
		t.Fatal(err)
	}
	p := &workerProcess{file: file}
	defer p.Close()
	f.MountErrorContract(t, p, func(fail bool) {
		p.poll = func(int32) (int16, error) {
			if fail {
				return 0, syscall.EACCES
			}
			return 0, nil
		}
		_, _ = p.Exited()
	})
}

func TestCheckoutDescriptorErrorsRetainSafeDiagnostic(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	w := &Workspace{roots: []*os.Root{root}}
	if _, failure := w.Source(); failure != port.MountUnavailable || w.MountError().Message == "" {
		t.Fatal("descriptor open error lost", failure, w.MountError())
	}
	file, err := os.CreateTemp(t.TempDir(), "descriptor")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	w.source = file
	if _, failure := w.Source(); failure != port.MountRefused || w.MountError().Message == "" {
		t.Fatal("descriptor stat error lost", failure, w.MountError())
	}
}

func TestMountedRootErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want port.Outcome
	}{
		{syscall.ENOENT, port.MountRootUnavailable},
		{syscall.ESRCH, port.MountRootUnavailable},
		{syscall.EACCES, port.MountRootUnavailable},
		{syscall.EIO, port.MountRefused},
		{syscall.EPERM, port.MountRefused},
		{errors.New("unknown stat failure"), port.MountRefused},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			w := &Workspace{MountStat: func(path string) (fs.FileInfo, error) {
				if path != "/proc/123/root/workspace" {
					t.Fatalf("stat path = %q", path)
				}
				return nil, &os.PathError{Op: "stat", Path: path, Err: tc.err}
			}}
			if mount, outcome := w.Mounted(123); mount != (port.MountSource{}) || outcome != tc.want || w.MountError() != mountSystemError(tc.err) {
				t.Fatal(mount, outcome, w.MountError())
			}
		})
	}
}
