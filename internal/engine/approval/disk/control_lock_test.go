package disk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

func TestControlPrivateLockRefusesUnsafeEvidence(t *testing.T) {
	for _, kind := range []string{"missing", "symlink", "hardlink", "mode", "directory", "owner", "open", "stat", "replace", "home readable"} {
		t.Run(kind, func(t *testing.T) {
			control, c, _, home, _ := controlFixture(t)
			tx, failure := control.Begin(t.Context(), c, true)
			if failure != "" {
				t.Fatal(failure)
			}
			tx.Close()
			path := filepath.Join(home, ".local/state/itos-container/coordination", controlLockName)
			ops := control.Store.operations()
			want := port.Failure("state.unsafe")
			switch kind {
			case "missing", "symlink", "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if kind == "missing" {
					want = "state.unavailable"
				}
				if kind == "symlink" {
					if err := os.Symlink(home, path); err != nil {
						t.Fatal(err)
					}
					want = "state.path"
				}
				if kind == "directory" {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
			case "hardlink":
				if err := os.Link(path, filepath.Join(home, "second-link")); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "owner":
				stat := ops.lstat
				ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
					info, err := stat(root, name)
					if err == nil && name == controlLockName {
						return controlWrongOwner{info}, nil
					}
					return info, err
				}
			case "open":
				ops.openRecord = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, os.ErrPermission }
				want = "state.unavailable"
			case "stat":
				stat := ops.statFile
				ops.statFile = func(file *os.File) (os.FileInfo, error) {
					if filepath.Base(file.Name()) == controlLockName {
						return nil, os.ErrPermission
					}
					return stat(file)
				}
				want = "state.path"
			case "replace":
				open := ops.openRecord
				ops.openRecord = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
					file, err := open(root, name, flags, mode)
					if name == controlLockName && err == nil {
						if err := os.Rename(path, filepath.Join(home, "old-lock")); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, nil, 0600); err != nil {
							t.Fatal(err)
						}
					}
					return file, err
				}
				want = "state.path"
			case "home readable":
				if err := os.Chmod(home, 0750); err != nil {
					t.Fatal(err)
				}
				want = ""
			}
			control.Store.ops = &ops
			tx, failure = control.Begin(t.Context(), c, false)
			if tx != nil {
				tx.Close()
			}
			if failure != want {
				t.Fatalf("got %s, want %s", failure, want)
			}
		})
	}
}

type controlWrongOwner struct{ os.FileInfo }

func (controlWrongOwner) Sys() any { return &struct{ Uid, Nlink uint64 }{^uint64(0), 1} }

func TestControlLockBootstrapFaults(t *testing.T) {
	for _, kind := range []string{"parent open", "directory open", "directory stat", "directory mode", "create", "chmod", "lookup", "lock sync", "racing create"} {
		t.Run(kind, func(t *testing.T) {
			control, c, _, home, _ := controlFixture(t)
			ops := control.Store.operations()
			bad := errors.New("private lock fault")
			want := port.Failure("state.write")
			switch kind {
			case "parent open":
				if err := os.MkdirAll(filepath.Join(home, ".local/state/itos-container/coordination"), 0700); err != nil {
					t.Fatal(err)
				}
				want = "state.unavailable"
				open := ops.openDir
				ops.openDir = func(root *os.Root, name string) (*os.File, error) {
					// Target the explicit descriptor validation, after pinPath.
					if filepath.Base(root.Name()) == "itos-container" {
						if _, err := root.Stat("coordination"); err == nil {
							return nil, bad
						}
					}
					return open(root, name)
				}
			case "directory open":
				if err := os.MkdirAll(filepath.Join(home, ".local/state/itos-container/coordination"), 0700); err != nil {
					t.Fatal(err)
				}
				open := ops.openPrivate
				ops.openPrivate = func(fd uintptr, name string) (int, error) {
					if name == "coordination" {
						return -1, bad
					}
					return open(fd, name)
				}
				want = "state.path"
			case "directory stat", "directory mode":
				if err := os.MkdirAll(filepath.Join(home, ".local/state/itos-container/coordination"), 0700); err != nil {
					t.Fatal(err)
				}
				stat := ops.statFile
				ops.statFile = func(file *os.File) (os.FileInfo, error) {
					if file.Name() == "coordination" {
						if kind == "directory mode" {
							if err := os.Chmod(filepath.Join(home, ".local/state/itos-container/coordination"), 0755); err != nil {
								t.Fatal(err)
							}
							return stat(file)
						}
						return nil, bad
					}
					return stat(file)
				}
				want = "state.path"
				if kind == "directory mode" {
					want = "state.unsafe"
				}
			case "create":
				ops.create = func(*os.Root, string) (*os.File, error) { return nil, bad }
			case "chmod":
				ops.chmod = func(*os.File, os.FileMode) error { return bad }
			case "lookup":
				stat := ops.lstat
				ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
					if name == controlLockName {
						return nil, bad
					}
					return stat(root, name)
				}
				want = "state.unavailable"
			case "lock sync":
				ops.syncFile = func(*os.File) error { return bad }
			case "racing create":
				create := ops.create
				ops.create = func(root *os.Root, name string) (*os.File, error) {
					file, err := create(root, name)
					if name == controlLockName && err == nil {
						_ = file.Close()
						return nil, os.ErrExist
					}
					return file, err
				}
				want = ""
			}
			control.Store.ops = &ops
			tx, failure := control.Begin(t.Context(), c, true)
			if tx != nil {
				tx.Close()
			}
			if failure != want {
				t.Fatalf("got %s, want %s", failure, want)
			}
		})
	}
}

func TestControlPrivateLockDeadlineAndReplacement(t *testing.T) {
	control, c, _, home, _ := controlFixture(t)
	tx, failure := control.Begin(t.Context(), c, true)
	if failure != "" {
		t.Fatal(failure)
	}
	defer tx.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	if other, f := control.Begin(ctx, c, false); other != nil || f != "state.busy" {
		t.Fatalf("contender: %v %s", other, f)
	}
	path := filepath.Join(home, ".local/state/itos-container/coordination", controlLockName)
	if err := os.Rename(path, filepath.Join(home, "retained-lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if tx.Unchanged() {
		t.Fatal("substituted lock retained publication authority")
	}
}

// Begin's lock wait is bounded by the shared call budget the harness substitutes
// at build time, never by a literal of its own: a worker that waits on a held
// lock gives up with the retryable state.busy at that budget, no sooner and no
// later. The fake clock makes the boundary exact without waiting for it.
func TestControlLockWaitEndsAtTheSharedCallBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		control, c, _, _, _ := controlFixture(t)
		peer, failure := control.Begin(t.Context(), c, true)
		if failure != "" {
			t.Fatal(failure)
		}
		defer peer.Close()
		for _, write := range []bool{false, true} {
			result := make(chan port.Failure, 1)
			go func() {
				tx, f := control.Begin(context.Background(), c, write)
				if tx != nil {
					tx.Close()
				}
				result <- f
			}()
			time.Sleep(budget.CallDuration - time.Millisecond)
			synctest.Wait()
			select {
			case f := <-result:
				t.Fatalf("write=%v gave up before the budget ended: %q", write, f)
			default:
			}
			time.Sleep(2 * time.Millisecond)
			synctest.Wait()
			select {
			case f := <-result:
				if f != "state.busy" {
					t.Fatalf("write=%v after the budget: %q, want state.busy", write, f)
				}
			default:
				t.Fatalf("write=%v still waiting after the budget ended", write)
			}
		}
	})
}

// The same budget also ends a discovery scan that outlasts it, whoever called.
func TestControlScanEndsAtTheSharedCallBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		control, c, reservations, _, _ := controlFixture(t)
		control.Reservations = &controlBudgetScan{Reservations: reservations, end: func() {}, failure: "state.path", slow: func() bool { return true }}
		result := make(chan port.Failure, 1)
		go func() {
			tx, f := control.Begin(context.Background(), c, false)
			if tx != nil {
				tx.Close()
			}
			result <- f
		}()
		time.Sleep(budget.CallDuration - time.Millisecond)
		synctest.Wait()
		select {
		case f := <-result:
			t.Fatalf("scan was given up before the budget ended: %q", f)
		default:
		}
		time.Sleep(2 * time.Millisecond)
		synctest.Wait()
		select {
		case f := <-result:
			if f != "state.busy" {
				t.Fatalf("scan after the budget: %q, want state.busy", f)
			}
		default:
			t.Fatal("scan still running after the budget ended")
		}
	})
}

func TestControlRacingBootstrapUsesOnePrivateInode(t *testing.T) {
	control, c, _, home, _ := controlFixture(t)
	ready, release := make(chan struct{}, 2), make(chan struct{})
	ops := control.Store.operations()
	mkdir := ops.mkdir
	ops.mkdir = func(root *os.Root, name string, mode os.FileMode) error {
		if name == ".local" {
			ready <- struct{}{}
			select {
			case <-release:
			case <-t.Context().Done():
				return t.Context().Err()
			}
		}
		return mkdir(root, name, mode)
	}
	control.Store.ops = &ops
	results := make(chan port.Failure, 2)
	for range 2 {
		go func() {
			tx, f := control.Begin(t.Context(), c, true)
			if tx != nil {
				tx.Close()
			}
			results <- f
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case f := <-results:
			close(release)
			t.Fatalf("creator never reached mkdir: %s", f)
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("creators did not meet at bootstrap")
		}
	}
	close(release)
	for range 2 {
		if f := <-results; f != "" {
			t.Fatal(f)
		}
	}
	path := filepath.Join(home, ".local/state/itos-container/coordination", controlLockName)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	tx, failure := control.Begin(t.Context(), c, true)
	if failure != "" {
		t.Fatal(failure)
	}
	tx.Close()
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("retry switched lock inode", err)
	}
}

func TestControlReadRechecksAbsentLock(t *testing.T) {
	control, c, _, _, _ := controlFixture(t)
	read, failure := control.Begin(t.Context(), c, false)
	if failure != "" {
		t.Fatal(failure)
	}
	defer read.Close()
	writer, failure := control.Begin(t.Context(), c, true)
	if failure != "" {
		t.Fatal(failure)
	}
	writer.Close()
	if read.Unchanged() {
		t.Fatal("unlocked inspection trusted concurrent bootstrap")
	}
}

type controlPeerReservations struct {
	port.Reservations
	selected string
	peer     func()
}

func (r *controlPeerReservations) Roots(ctx context.Context, c port.Candidate, path string) ([]string, port.Failure) {
	roots, failure := r.Reservations.Roots(ctx, c, path)
	if path == r.selected && r.peer != nil {
		peer := r.peer
		r.peer = nil
		peer()
	}
	return roots, failure
}

func TestControlPeerPublishesAfterUnlockedInspection(t *testing.T) {
	for _, kind := range []string{"writer", "existing directory", "reader", "new reservation", "unsafe lock", "replaced directory"} {
		t.Run(kind, func(t *testing.T) {
			control, c, reservations, home, selected := controlFixture(t)
			peer := control
			path := filepath.Join(home, ".local/state/itos-container/coordination")
			if kind == "existing directory" || kind == "replaced directory" {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			var lockInfo os.FileInfo
			control.Reservations = &controlPeerReservations{Reservations: reservations, selected: selected, peer: func() {
				tx, f := peer.Begin(t.Context(), c, true)
				if f != "" {
					t.Fatal("peer bootstrap:", f)
				}
				tx.Close()
				var err error
				lockInfo, err = os.Stat(filepath.Join(path, controlLockName))
				if err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "new reservation":
					reservations.RootsByLocation[selected] = []string{filepath.Dir(selected)}
				case "unsafe lock":
					if err := os.Chmod(filepath.Join(path, controlLockName), 0644); err != nil {
						t.Fatal(err)
					}
				case "replaced directory":
					if err := os.Rename(path, path+"-old"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}}
			want := port.Failure("")
			switch kind {
			case "reader", "replaced directory":
				want = "state.path"
			case "new reservation":
				want = "state.exposed"
			case "unsafe lock":
				want = "state.unsafe"
			}
			tx, failure := control.Begin(t.Context(), c, kind != "reader")
			if tx != nil {
				tx.Close()
			}
			if failure != want {
				t.Fatalf("writer after peer publication: %s, want %s", failure, want)
			}
			if want == "" {
				after, err := os.Stat(filepath.Join(path, controlLockName))
				if err != nil || !os.SameFile(lockInfo, after) {
					t.Fatalf("writer replaced peer lock: %v", err)
				}
			}
		})
	}
}

// flock(2): on NFS a descriptor needs write access to hold an exclusive lock,
// and read access is enough for a shared one. The lock file is private, so
// opening it for writing is open only to its owner.
func TestControlLockAccessModeFollowsLockMode(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(fmt.Sprint("write=", write), func(t *testing.T) {
			control, c, _, _, _ := controlFixture(t)
			tx, failure := control.Begin(t.Context(), c, true)
			if failure != "" {
				t.Fatal(failure)
			}
			tx.Close()
			ops := control.Store.operations()
			openRecord := ops.openRecord
			var modes []int
			var creation []os.FileMode
			ops.openRecord = func(root *os.Root, name string, flag int, perm os.FileMode) (*os.File, error) {
				if name == controlLockName {
					modes = append(modes, flag&(os.O_RDONLY|os.O_WRONLY|os.O_RDWR))
					creation = append(creation, perm)
				}
				return openRecord(root, name, flag, perm)
			}
			control.Store.ops = &ops
			tx, failure = control.Begin(t.Context(), c, write)
			if failure != "" {
				t.Fatal(failure)
			}
			tx.Close()
			want := os.O_RDONLY
			if write {
				want = os.O_RDWR
			}
			if len(modes) != 1 || modes[0] != want {
				t.Fatalf("lock opened with access modes %v, want [%d]", modes, want)
			}
			// The existing lock was validated before this open, so the open names no
			// creation mode: only the exclusive create in acquire may set one.
			if len(creation) != 1 || creation[0] != 0 {
				t.Fatalf("existing lock opened with creation modes %v, want [0]", creation)
			}
		})
	}
}

// A peer can create the lock after the writer saw it absent but before the
// writer's unlocked catalog scan reads what the peer published. The scan then
// sees names the peer wrote under a lock the writer does not hold. That is not
// substitution: the writer keeps its pinned directory checks, takes the peer's
// lock and repeats discovery under it.
func TestControlWriterJoinsPeerLockCreatedDuringCatalogScan(t *testing.T) {
	for _, kind := range []string{"published catalog", "unpublished temporary binding"} {
		t.Run(kind, func(t *testing.T) {
			control, c, _, home, _ := controlFixture(t)
			peer := control
			path := filepath.Join(home, ".local/state/itos-container/coordination")
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			ops := control.Store.operations()
			lstat := ops.lstat
			fired := false
			ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
				info, err := lstat(root, name)
				if name != controlLockName || !errors.Is(err, os.ErrNotExist) || fired {
					return info, err
				}
				fired = true
				switch kind {
				case "published catalog":
					tx, failure := peer.Begin(t.Context(), c, true)
					if failure != "" {
						t.Fatal("peer bootstrap:", failure)
					}
					tx.Close()
				case "unpublished temporary binding":
					for _, name := range []string{controlLockName, ".location-" + strings.Repeat("0", 32) + ".tmp"} {
						if err := os.WriteFile(filepath.Join(path, name), nil, 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
				return info, err
			}
			control.Store.ops = &ops
			tx, failure := control.Begin(t.Context(), c, true)
			if tx != nil {
				tx.Close()
			}
			if failure != "" {
				t.Fatalf("writer racing a peer's unlocked scan: %s", failure)
			}
			if !fired {
				t.Fatal("the peer never ran")
			}
			if _, err := os.Stat(filepath.Join(path, "self.json")); err != nil {
				t.Fatalf("writer did not publish its self-binding under the peer's lock: %v", err)
			}
		})
	}
}

// A peer's lock must not hide a refusal. A reader never takes the lock, so it
// reports the refusal its own inspection found; a writer's provisional refusal
// is found again under the lock it then takes.
func TestControlPeerLockDoesNotHideRefusals(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(fmt.Sprint("write=", write), func(t *testing.T) {
			control, c, reservations, home, _ := controlFixture(t)
			path := filepath.Join(home, ".local/state/itos-container/coordination")
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			reservations.Failure = "state.unavailable"
			ops := control.Store.operations()
			lstat := ops.lstat
			fired := false
			ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
				info, err := lstat(root, name)
				if name == controlLockName && errors.Is(err, os.ErrNotExist) && !fired {
					fired = true
					if err := os.WriteFile(filepath.Join(path, controlLockName), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
				return info, err
			}
			control.Store.ops = &ops
			tx, failure := control.Begin(t.Context(), c, write)
			if tx != nil {
				tx.Close()
			}
			if failure != "state.unavailable" || !fired {
				t.Fatalf("refusal beside a peer's lock: %q (peer ran: %v), want state.unavailable", failure, fired)
			}
		})
	}
}

func TestControlBootstrapRechecksReservationsUnderPrivateLock(t *testing.T) {
	control, c, reservations, _, selected := controlFixture(t)
	ops := control.Store.operations()
	create := ops.create
	ops.create = func(root *os.Root, name string) (*os.File, error) {
		file, err := create(root, name)
		if name == controlLockName && err == nil {
			reservations.RootsByLocation[selected] = []string{filepath.Dir(selected)}
		}
		return file, err
	}
	control.Store.ops = &ops
	tx, failure := control.Begin(t.Context(), c, true)
	if tx != nil {
		tx.Close()
	}
	if failure != "state.unavailable" {
		t.Fatalf("lost legacy evidence during bootstrap: %s", failure)
	}
	if _, err := os.Stat(selected); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrote ordinary state without separation: %v", err)
	}
}
