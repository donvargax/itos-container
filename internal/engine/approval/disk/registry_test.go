package disk

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

func TestRegistryStorageContract(t *testing.T) {
	for _, backend := range []string{"memory", "disk"} {
		t.Run(backend, func(t *testing.T) {
			open := func() (port.RegistryStorage, port.Candidate) {
				s, c, _ := diskFixture(t)
				if backend == "memory" {
					return &porttest.Registry{}, c
				}
				return Registry{Store: s}, c
			}
			porttest.RegistryStorageContract(t, open)
			porttest.RegistryReadersContract(t, open)
		})
	}
}

func TestRegistryRefusesUnsafeLocksAndRecords(t *testing.T) {
	for _, defect := range []string{"controller symlink", "operation symlink", "record symlink", "record mode", "directory mode", "hard link", "replaced lock"} {
		t.Run(defect, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			r := Registry{Store: s}
			id := strings.Repeat("a", 64)
			ctx := context.Background()
			lease, f := r.Register(ctx, c, id, []byte("record"))
			if f != "" {
				t.Fatal(f)
			}
			defer lease.Close()
			path, _ := s.locationAt(c, "sessions")
			path = filepath.Join(path, id)
			switch defect {
			case "controller symlink", "operation symlink":
				name := strings.Fields(defect)[0] + ".lock"
				if err := os.Rename(filepath.Join(path, name), filepath.Join(path, "old")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("old", filepath.Join(path, name)); err != nil {
					t.Fatal(err)
				}
			case "record symlink":
				if err := os.Rename(filepath.Join(path, "record.json"), filepath.Join(path, "old")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("old", filepath.Join(path, "record.json")); err != nil {
					t.Fatal(err)
				}
			case "record mode":
				if err := os.Chmod(filepath.Join(path, "record.json"), 0644); err != nil {
					t.Fatal(err)
				}
			case "directory mode":
				if err := os.Chmod(path, 0755); err != nil {
					t.Fatal(err)
				}
			case "hard link":
				if err := os.Link(filepath.Join(path, "record.json"), filepath.Join(path, "linked")); err != nil {
					t.Fatal(err)
				}
			case "replaced lock":
				d, f := r.Open(ctx, c, id, false)
				if f != "" {
					t.Fatal(f)
				}
				defer d.Close()
				if err := os.Rename(filepath.Join(path, "operation.lock"), filepath.Join(path, "old")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "operation.lock"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				if d.Unchanged() {
					t.Fatal("replacement retained lock authority")
				}
				return
			}
			d, f := r.Open(ctx, c, id, false)
			if f == "" && d != nil {
				defer d.Close()
				_, f = d.Read("record.json")
			}
			if f != "state.unsafe" && f != "state.path" {
				t.Fatal("unsafe object accepted", f)
			}
		})
	}
}

func TestRegistryLockBudgetAndIndependentSession(t *testing.T) {
	s, c, _ := diskFixture(t)
	r := Registry{Store: s}
	ctx := context.Background()
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, id := range []string{first, second} {
		lease, f := r.Register(ctx, c, id, []byte("record"))
		if f != "" {
			t.Fatal(f)
		}
		defer lease.Close()
	}
	d, f := r.Open(ctx, c, first, false)
	if f != "" {
		t.Fatal(f)
	}
	defer d.Close()
	budget, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if other, f := r.Open(budget, c, first, false); other != nil || f != "state.unavailable" {
		t.Fatal("contended lock escaped deadline", f)
	}
	other, f := r.Open(ctx, c, second, false)
	if f != "" {
		t.Fatal("unrelated session blocked", f)
	}
	other.Close()
}

func TestRegistryPublicationFailuresNeverReturnLease(t *testing.T) {
	for _, fault := range []string{"write", "file sync", "parent sync", "directory sync"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			ioFault := SnapshotIO{}
			broken := func(*os.File) error { return errors.New("fixture failure") }
			switch fault {
			case "write":
				ioFault.Write = func(*os.File, []byte) (int, error) { return 0, io.ErrShortWrite }
			case "file sync":
				ioFault.FileSync = broken
			case "parent sync":
				ioFault.ParentSync = broken
			case "directory sync":
				ioFault.DirectorySync = broken
			}
			lease, f := (Registry{Store: s, IO: ioFault}).Register(context.Background(), c, strings.Repeat("a", 64), []byte("record"))
			if lease != nil || f != "state.write" {
				t.Fatal("failed durability gave a controller lease", f)
			}
		})
	}
}
