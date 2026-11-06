package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func TestRegistryRefusesFailedLockPublication(t *testing.T) {
	for _, fault := range []string{"open", "chmod", "stat", "nonempty", "replaced", "parent sync", "operation open", "controller contention", "exclusive publication"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			ops := s.operations()
			var held *os.File
			t.Cleanup(func() {
				if held != nil {
					_ = held.Close()
				}
			})
			broken := errors.New("lock storage unavailable")
			originalOpen := ops.openRecord
			ops.openRecord = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
				if (fault == "open" && name == "controller.lock") || (fault == "operation open" && name == "operation.lock") {
					return nil, broken
				}
				f, err := originalOpen(root, name, flags, mode)
				if err == nil && name == "controller.lock" {
					switch fault {
					case "nonempty":
						_, err = f.Write([]byte("x"))
					case "replaced":
						err = root.Rename(name, name+".original")
					case "controller contention":
						held, err = root.Open(name)
						if err == nil {
							err = tryLock(held)
						}
					}
				}
				return f, err
			}
			originalChmod := ops.chmod
			ops.chmod = func(f *os.File, mode os.FileMode) error {
				if fault == "chmod" && strings.HasSuffix(f.Name(), "controller.lock") {
					return broken
				}
				return originalChmod(f, mode)
			}
			if fault == "stat" {
				ops.statFile = func(*os.File) (os.FileInfo, error) { return nil, broken }
			}
			if fault == "parent sync" {
				ops.syncParent = func(*os.File) error { return broken }
			}
			s.ops = &ops
			r := Registry{Store: s}
			if fault == "exclusive publication" {
				r.Exclusive = func(*os.Root, string, string) error { return broken }
			}
			lease, f := r.Register(t.Context(), c, strings.Repeat("a", 64), []byte("record"))
			want := map[string]port.Failure{"open": "state.path", "chmod": "state.write", "stat": "state.unsafe", "nonempty": "state.unsafe", "replaced": "state.path", "parent sync": "state.write", "operation open": "state.path", "controller contention": "state.conflict", "exclusive publication": "state.write"}[fault]
			if lease != nil || f != want {
				t.Fatalf("failed %s returned lease %v, failure %s; want %s", fault, lease, f, want)
			}
		})
	}
}

func TestRegistryLockValidationAndReadOnlyFailures(t *testing.T) {
	for _, fault := range []string{"missing controller", "missing operation", "nonempty controller", "nonempty operation", "changed directory", "closed controller"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			r := Registry{Store: s}
			id := strings.Repeat("a", 64)
			lease, f := r.Register(t.Context(), c, id, []byte("original"))
			if f != "" {
				t.Fatal(f)
			}
			lease.Close()
			entry, f := r.Open(t.Context(), c, id, false)
			if f != "" {
				t.Fatal(f)
			}
			defer entry.Close()
			path, _ := s.locationAt(c, "sessions")
			path = filepath.Join(path, id)
			var err error
			switch fault {
			case "missing controller", "missing operation":
				err = os.Remove(filepath.Join(path, strings.Fields(fault)[1]+".lock"))
			case "nonempty controller", "nonempty operation":
				err = os.WriteFile(filepath.Join(path, strings.Fields(fault)[1]+".lock"), []byte("x"), 0600)
			case "changed directory":
				err = os.Chmod(path, 0755)
			case "closed controller":
				err = entry.(*sessionDirectory).controller.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			if entry.Unchanged() || entry.Publish("record.json", []byte("original"), []byte("changed")) != "state.path" || entry.Sync("record.json", []byte("original")) != "state.path" {
				t.Fatal("unsafe lock still authorized publication or durability repair")
			}
			if state, f := entry.Controller(); state != "unknown" || f != "state.path" {
				t.Fatal("unsafe lock produced a controller observation", state, f)
			}
			entry.Close()
			if fault != "closed controller" {
				if next, f := r.Open(t.Context(), c, id, true); next != nil || f == "" {
					t.Fatal("fresh reader accepted unsafe locks", next, f)
				}
			}
		})
	}
}

func TestRegistryRejectsInvalidPathsAndCanceledOperations(t *testing.T) {
	s, c, _ := diskFixture(t)
	r := Registry{Store: s}
	if lease, f := r.Register(t.Context(), c, "../other", nil); lease != nil || f != "state.invalid" {
		t.Fatal(lease, f)
	}
	if entry, f := r.Open(t.Context(), c, "../other", false); entry != nil || f != "state.invalid" {
		t.Fatal(entry, f)
	}
	bad := c
	bad.Project = "../other"
	if entry, f := r.Open(t.Context(), bad, strings.Repeat("a", 64), false); entry != nil || f != "state.invalid" {
		t.Fatal(entry, f)
	}
	if ids, f := r.List(t.Context(), bad); ids != nil || f != "state.invalid" {
		t.Fatal(ids, f)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	file, err := os.CreateTemp(t.TempDir(), "lock")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := sessionLock(ctx, file, true); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled session lock acquired", err)
	}
	_ = file.Close()
	if err := sessionLock(t.Context(), file, false); err == nil {
		t.Fatal("closed lock acquired")
	}
}
