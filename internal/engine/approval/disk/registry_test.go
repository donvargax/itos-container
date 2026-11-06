package disk

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	for _, defect := range []string{"controller symlink", "operation symlink", "controller size", "operation size", "record symlink", "record mode", "directory mode", "hard link", "replaced lock"} {
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
			case "controller size", "operation size":
				name := strings.Fields(defect)[0] + ".lock"
				if err := os.WriteFile(filepath.Join(path, name), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
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
	for _, fault := range []string{"create", "write", "file sync", "parent sync", "directory sync"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			ioFault := SnapshotIO{}
			broken := func(*os.File) error { return errors.New("fixture failure") }
			switch fault {
			case "create":
				ioFault.Create = func(*os.Root, string) (*os.File, error) { return nil, errors.New("fixture create failure") }
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

func TestRegistryListRetainsNamesAndRejectsUnavailableEvidence(t *testing.T) {
	for _, fault := range []string{"none", "read directory", "revalidate directory"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			ctx := context.Background()
			ids := []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}
			for _, id := range ids {
				lease, failure := (Registry{Store: s}).Register(ctx, c, id, []byte("record"))
				if failure != "" {
					t.Fatal(failure)
				}
				lease.Close()
			}
			path, _ := s.locationAt(c, "sessions")
			ops := s.operations()
			openDir, statRoot := ops.openDir, ops.statRoot
			var listed *os.File
			checks := 0
			ops.openDir = func(root *os.Root, name string) (*os.File, error) {
				file, err := openDir(root, name)
				if root.Name() == path && name == "." {
					listed = file
				}
				return file, err
			}
			ops.statRoot = func(root *os.Root, name string) (os.FileInfo, error) {
				info, err := statRoot(root, name)
				if root.Name() == path && listed != nil {
					checks++
					if checks == 1 && fault == "read directory" {
						if closeErr := listed.Close(); closeErr != nil {
							t.Fatal(closeErr)
						}
					}
					if checks == 2 && fault == "revalidate directory" {
						return nil, os.ErrNotExist
					}
				}
				return info, err
			}
			s.ops = &ops
			got, failure := (Registry{Store: s}).List(ctx, c)
			want := map[string]port.Failure{"none": "", "read directory": "state.unavailable", "revalidate directory": "state.path"}[fault]
			if failure != want || (want != "" && got != nil) {
				t.Fatalf("list = %v, %s; want %s", got, failure, want)
			}
			if fault == "none" {
				slices.Sort(got)
				if !slices.Equal(got, ids) {
					t.Fatal("lost registered sessions", got)
				}
			}
		})
	}
}

func TestRegistryRegistrationFaultsRetainNoControllerLease(t *testing.T) {
	for _, fault := range []string{"invalid id", "lookup", "mkdir", "controller open", "operation open", "chmod", "stat", "occupied controller", "changed before publication", "cancelled after publication"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			id := strings.Repeat("b", 64)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ops := s.operations()
			lookup, mkdir, openRecord := ops.lstat, ops.mkdir, ops.openRecord
			faultErr := errors.New("registry fixture fault")
			want := port.Failure("state.path")
			r := Registry{}
			switch fault {
			case "invalid id":
				id, want = "../foreign", "state.invalid"
			case "lookup":
				want = "state.unavailable"
				ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
					if name == id {
						return nil, faultErr
					}
					return lookup(root, name)
				}
			case "mkdir":
				want = "state.write"
				ops.mkdir = func(root *os.Root, name string, mode os.FileMode) error {
					if name == id {
						return faultErr
					}
					return mkdir(root, name, mode)
				}
			case "controller open", "operation open":
				ops.openRecord = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
					if name == strings.Fields(fault)[0]+".lock" {
						return nil, faultErr
					}
					return openRecord(root, name, flags, mode)
				}
			case "chmod":
				want = "state.write"
				ops.chmod = func(*os.File, os.FileMode) error { return faultErr }
			case "stat":
				want = "state.unsafe"
				r.Stat = func(file *os.File) (os.FileInfo, error) {
					if strings.HasSuffix(file.Name(), ".lock") {
						return nil, faultErr
					}
					return file.Stat()
				}
			case "occupied controller":
				want = "state.conflict"
				ops.openRecord = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
					file, err := openRecord(root, name, flags, mode)
					if err == nil && name == "controller.lock" {
						holder, err := root.OpenFile(name, os.O_RDWR, 0)
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = holder.Close() })
						if err := tryLock(holder); err != nil {
							t.Fatal(err)
						}
					}
					return file, err
				}
			case "changed before publication":
				ops.syncParent = func(dir *os.File) error {
					path := filepath.Join(dir.Name(), "operation.lock")
					if _, err := os.Stat(path); err == nil {
						if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					return dir.Sync()
				}
			case "cancelled after publication":
				want = "state.unavailable"
				published := false
				ops.syncParent = func(dir *os.File) error {
					if _, err := os.Stat(filepath.Join(dir.Name(), "record.json")); err == nil {
						published = true
					}
					return dir.Sync()
				}
				statRoot := ops.statRoot
				ops.statRoot = func(root *os.Root, name string) (os.FileInfo, error) {
					info, err := statRoot(root, name)
					if published && filepath.Base(root.Name()) == id {
						cancel()
					}
					return info, err
				}
			}
			s.ops = &ops
			r.Store = s
			lease, failure := r.Register(ctx, c, id, []byte("record"))
			if lease != nil {
				lease.Close()
			}
			if lease != nil || failure != want {
				t.Fatalf("register = %v, %s; want no lease and %s", lease, failure, want)
			}
		})
	}
}

func TestRegistryOpenRejectsInvalidOrMissingLocks(t *testing.T) {
	for _, fault := range []string{"invalid id", "invalid location", "missing controller", "missing operation", "changed after lock"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			id := strings.Repeat("a", 64)
			ctx := context.Background()
			r := Registry{Store: s}
			lease, failure := r.Register(ctx, c, id, []byte("record"))
			if failure != "" {
				t.Fatal(failure)
			}
			lease.Close()
			path, _ := s.locationAt(c, "sessions")
			path = filepath.Join(path, id)
			want := port.Failure("state.invalid")
			switch fault {
			case "invalid id":
				id = "invalid"
			case "invalid location":
				r.Store.StateHome, want = "relative", "state.location"
			case "missing controller", "missing operation":
				if err := os.Remove(filepath.Join(path, strings.Fields(fault)[1]+".lock")); err != nil {
					t.Fatal(err)
				}
			case "changed after lock":
				want = "state.path"
				ops := s.operations()
				lookup := ops.lstat
				ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
					info, err := lookup(root, name)
					if name == "operation.lock" {
						if err := os.Chmod(path, 0755); err != nil {
							t.Fatal(err)
						}
					}
					return info, err
				}
				r.Store.ops = &ops
			}
			d, failure := r.Open(ctx, c, id, false)
			if d != nil {
				d.Close()
			}
			if d != nil || failure != want {
				t.Fatalf("open = %v, %s; want %s", d, failure, want)
			}
		})
	}
}

func TestRegistrySessionRejectsChangedAuthorityAndFailedPublication(t *testing.T) {
	for _, fault := range []string{"cancelled", "lock grew", "rename", "controller probe", "controller revalidation"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			id := strings.Repeat("a", 64)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := Registry{Store: s}
			lease, failure := r.Register(ctx, c, id, []byte("record"))
			if failure != "" {
				t.Fatal(failure)
			}
			lease.Close()
			if fault == "rename" {
				r.IO.Rename = func(*os.Root, string, string) error { return errors.New("rename unavailable") }
			}
			opened, failure := r.Open(ctx, c, id, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer opened.Close()
			d := opened.(*sessionDirectory)
			// During lock assembly, unset handles do not invalidate the pinned
			// directory; every installed handle is subsequently checked.
			assembling := sessionDirectory{directoryStore: d.directoryStore}
			if !assembling.Unchanged() {
				t.Fatal("safe pin without lock handles was rejected")
			}
			switch fault {
			case "cancelled":
				cancel()
			case "lock grew":
				if err := os.WriteFile(d.controller.Name(), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			case "rename":
				if failure := d.Publish("record.json", []byte("record"), []byte("next")); failure != "state.write" {
					t.Fatal(failure)
				}
				data, failure := d.Read("record.json")
				if failure != "" || string(data) != "record" {
					t.Fatal("failed rename lost original evidence", failure)
				}
				return
			case "controller probe", "controller revalidation":
				lookup := d.pin.ops.lstat
				d.pin.ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
					info, err := lookup(root, name)
					if name == "controller.lock" {
						if fault == "controller probe" {
							_ = d.controller.Close()
						} else {
							cancel()
						}
					}
					return info, err
				}
				state, failure := d.Controller()
				if state != "unknown" || failure != "state.unavailable" {
					t.Fatal(state, failure)
				}
				return
			}
			if d.Unchanged() {
				t.Fatal("changed authority remained valid")
			}
			if failure := d.Publish("record.json", []byte("record"), []byte("next")); failure != "state.path" {
				t.Fatal(failure)
			}
			if failure := d.Sync("record.json", []byte("record")); failure != "state.path" {
				t.Fatal(failure)
			}
			if state, failure := d.Controller(); state != "unknown" || failure != "state.path" {
				t.Fatal(state, failure)
			}
		})
	}
}

func TestSessionLockRejectsExpiredContextBeforeAcquiring(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "operation")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sessionLock(ctx, file, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
