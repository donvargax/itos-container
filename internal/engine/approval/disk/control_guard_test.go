package disk

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

func guardCatalog(home string) string {
	return filepath.Join(home, ".local/state/itos-container/coordination")
}

// substituteLock gives control.lock a new inode, as a hostile peer replacing it would.
func substituteLock(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(guardCatalog(home), controlLockName)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
}

func once(fn func()) func() {
	done := false
	return func() {
		if !done {
			done = true
			fn()
		}
	}
}

func beginRefused(t *testing.T, control Control, c port.Candidate, write bool, want port.Failure) {
	t.Helper()
	tx, failure := control.Begin(t.Context(), c, write)
	if tx != nil {
		tx.Close()
	}
	if failure != want {
		t.Fatalf("write=%v: %q, want %q", write, failure, want)
	}
}

func bootstrapped(t *testing.T) (Control, port.Candidate, *porttest.Reservations, string, string) {
	t.Helper()
	control, c, reservations, home, selected := controlFixture(t)
	tx, failure := control.Begin(t.Context(), c, true)
	if failure != "" {
		t.Fatal(failure)
	}
	tx.Close()
	return control, c, reservations, home, selected
}

// pending leaves the selected location's claim published but incomplete, as an
// interrupted enrollment does.
func pending(t *testing.T) (Control, port.Candidate, string, string) {
	t.Helper()
	control, c, _, home, selected := controlFixture(t)
	ops := control.Store.operations()
	mkdir := ops.mkdir
	ops.mkdir = func(root *os.Root, name string, mode os.FileMode) error {
		if filepath.Join(root.Name(), name) == selected {
			return os.ErrPermission
		}
		return mkdir(root, name, mode)
	}
	control.Store.ops = &ops
	beginRefused(t, control, c, true, "state.write")
	control.Store.ops = nil
	return control, c, home, selected
}

type hookedReservations struct {
	port.Reservations
	hook func()
}

func (r hookedReservations) Roots(ctx context.Context, c port.Candidate, path string) ([]string, port.Failure) {
	r.hook()
	return r.Reservations.Roots(ctx, c, path)
}

func TestControlInspectionRefusesUnsafeSelectedTrees(t *testing.T) {
	for _, kind := range []string{"selected tree", "evidence collection", "unreadable evidence collection"} {
		t.Run(kind, func(t *testing.T) {
			control, c, _, _, selected := controlFixture(t)
			want := port.Failure("state.unsafe")
			mode := os.FileMode(0700)
			if kind == "selected tree" {
				mode = 0755
			}
			if err := os.MkdirAll(selected, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(selected, mode); err != nil {
				t.Fatal(err)
			}
			if kind != "selected tree" {
				collection := filepath.Join(selected, "intents")
				if err := os.Mkdir(collection, 0700); err != nil {
					t.Fatal(err)
				}
				if kind == "evidence collection" {
					if err := os.Chmod(collection, 0755); err != nil {
						t.Fatal(err)
					}
				} else {
					// A collection that can no longer be listed is unavailable, not empty.
					ops := control.Store.operations()
					open := ops.openDir
					ops.openDir = func(root *os.Root, name string) (*os.File, error) {
						file, err := open(root, name)
						if err == nil && name == "." && filepath.Base(root.Name()) == "intents" {
							_ = file.Close()
						}
						return file, err
					}
					control.Store.ops = &ops
					want = "state.unavailable"
				}
			}
			for _, write := range []bool{false, true} {
				beginRefused(t, control, c, write, want)
			}
		})
	}
}

func TestControlRefusesCompleteBindingWhoseLocationIsGone(t *testing.T) {
	control, c, _, home, selected := bootstrapped(t)
	if err := os.RemoveAll(selected); err != nil {
		t.Fatal(err)
	}
	// A binding claiming a complete location while listing only the ancestors that
	// still exist can only be edited, never written by a command.
	path := filepath.Join(guardCatalog(home), locationName(selected))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var binding locationBinding
	if err := json.Unmarshal(data, &binding); err != nil {
		t.Fatal(err)
	}
	binding.Ancestors = binding.Ancestors[:len(strings.Split(strings.Trim(filepath.Dir(selected), "/"), "/"))]
	edited, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, edited, 0600); err != nil {
		t.Fatal(err)
	}
	for _, write := range []bool{false, true} {
		beginRefused(t, control, c, write, "state.unavailable")
	}
}

func TestControlRefusesBindingThatCannotBeOpened(t *testing.T) {
	control, c, _, _, _ := bootstrapped(t)
	ops := control.Store.operations()
	open := ops.openRecord
	ops.openRecord = func(root *os.Root, name string, flag int, perm os.FileMode) (*os.File, error) {
		if strings.HasSuffix(name, ".json") {
			return nil, errors.New("binding removed after inspection")
		}
		return open(root, name, flag, perm)
	}
	control.Store.ops = &ops
	beginRefused(t, control, c, false, "state.unavailable")
}

func TestControlRefusesSubstitutionBetweenEnrollmentSteps(t *testing.T) {
	for _, kind := range []string{"before enrollment", "before self-binding", "before binding rename", "after binding rename",
		"before binding sync", "binding unreadable at sync", "binding rewritten before sync", "before pending completion"} {
		t.Run(kind, func(t *testing.T) {
			var control Control
			var c port.Candidate
			var home, selected string
			var reservations *porttest.Reservations
			if strings.HasPrefix(kind, "before binding") && kind != "before binding rename" || kind == "binding unreadable at sync" || kind == "binding rewritten before sync" {
				control, c, reservations, home, selected = bootstrapped(t)
			} else if kind == "before pending completion" {
				control, c, home, selected = pending(t)
			} else {
				control, c, reservations, home, selected = controlFixture(t)
			}
			ops := control.Store.operations()
			want := port.Failure("state.path")
			substitute := once(func() { substituteLock(t, home) })
			locked := func() bool {
				_, err := os.Lstat(filepath.Join(guardCatalog(home), controlLockName))
				return err == nil
			}
			whenLocked := func() {
				if locked() {
					substitute()
				}
			}
			self := filepath.Join(guardCatalog(home), "self.json")
			switch kind {
			case "before enrollment":
				control.Reservations = hookedReservations{Reservations: reservations, hook: whenLocked}
			case "before self-binding":
				ops.syncDir = func(file *os.File) error { whenLocked(); return file.Sync() }
			case "before binding rename":
				closeFile := ops.closeFile
				ops.closeFile = func(file *os.File) error { whenLocked(); return closeFile(file) }
			case "after binding rename":
				ops.syncParent = func(file *os.File) error { whenLocked(); return file.Sync() }
			case "before binding sync":
				ops.syncDir = func(file *os.File) error { substitute(); return file.Sync() }
			case "binding unreadable at sync":
				ops.syncDir = func(file *os.File) error {
					once(func() {
						if err := os.Chmod(self, 0644); err != nil {
							t.Fatal(err)
						}
					})()
					return file.Sync()
				}
				want = "state.unsafe"
			case "binding rewritten before sync":
				rewrite := once(func() {
					data, err := os.ReadFile(self)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(self, append(data, '\n'), 0600); err != nil {
						t.Fatal(err)
					}
				})
				ops.syncDir = func(file *os.File) error { rewrite(); return file.Sync() }
				want = "state.invalid"
			case "before pending completion":
				syncFile := ops.syncFile
				ops.syncFile = func(file *os.File) error {
					if filepath.Base(file.Name()) == locationName(selected) {
						substitute()
					}
					return syncFile(file)
				}
			}
			control.Store.ops = &ops
			beginRefused(t, control, c, true, want)
		})
	}
}

func TestControlEnrollmentFaultsAfterBootstrap(t *testing.T) {
	bad := errors.New("fault after bootstrap")
	for _, kind := range []string{"catalog directory sync", "catalog directory open", "binding create", "binding chmod", "selected lookup", "pending binding publication", "pending directory sync"} {
		t.Run(kind, func(t *testing.T) {
			var control Control
			var c port.Candidate
			var selected string
			switch kind {
			case "binding create", "binding chmod":
				control, c, _, _, selected = controlFixture(t)
			case "pending binding publication", "pending directory sync":
				control, c, _, selected = pending(t)
			default:
				control, c, _, _, selected = bootstrapped(t)
			}
			ops := control.Store.operations()
			want := port.Failure("state.write")
			switch kind {
			case "catalog directory sync":
				ops.syncDir = func(*os.File) error { return bad }
			case "catalog directory open":
				// Fail the directory opened for syncing, not one pinned for reading: the
				// lock sync that precedes it arms the fault.
				armed := false
				syncFile, openDir := ops.syncFile, ops.openDir
				ops.syncFile = func(file *os.File) error { armed = true; return syncFile(file) }
				ops.openDir = func(root *os.Root, name string) (*os.File, error) {
					if armed {
						return nil, bad
					}
					return openDir(root, name)
				}
			case "binding create":
				create := ops.create
				ops.create = func(root *os.Root, name string) (*os.File, error) {
					if strings.HasPrefix(name, ".location-") {
						return nil, bad
					}
					return create(root, name)
				}
			case "binding chmod":
				chmod := ops.chmod
				ops.chmod = func(file *os.File, mode os.FileMode) error {
					if strings.HasPrefix(filepath.Base(file.Name()), ".location-") {
						return bad
					}
					return chmod(file, mode)
				}
			case "selected lookup":
				// Another selection has no binding, so enrollment inspects it afresh
				// after the catalog was synced.
				control.Store.StateHome = filepath.Join(filepath.Dir(filepath.Dir(selected)), "other")
				armed := false
				lstat := ops.lstat
				syncDir := ops.syncDir
				ops.syncDir = func(file *os.File) error { armed = true; return syncDir(file) }
				ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
					if armed && name == "other" {
						return nil, bad
					}
					return lstat(root, name)
				}
				want = "state.unavailable"
			case "pending binding publication":
				rename := ops.rename
				ops.rename = func(root *os.Root, from, to string) error {
					if to == locationName(selected) {
						return bad
					}
					return rename(root, from, to)
				}
			case "pending directory sync":
				// The directory sync after the selected tree exists is the last one the
				// enrollment makes, so a dry run counts them and the real run fails it.
				var calls atomic.Int64
				count := ops
				count.syncDir = func(file *os.File) error { calls.Add(1); return file.Sync() }
				dry, _, _, _ := pending(t)
				dry.Store.ops = &count
				tx, failure := dry.Begin(t.Context(), c, true)
				if failure != "" {
					t.Fatal("dry run:", failure)
				}
				tx.Close()
				last, seen := calls.Load(), atomic.Int64{}
				ops.syncDir = func(file *os.File) error {
					if seen.Add(1) == last {
						return bad
					}
					return file.Sync()
				}
			}
			control.Store.ops = &ops
			beginRefused(t, control, c, true, want)
		})
	}
}

func TestControlRefusesLockSubstitutedWhileWaiting(t *testing.T) {
	control, c, _, home, _ := bootstrapped(t)
	held, err := os.OpenFile(filepath.Join(guardCatalog(home), controlLockName), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	if err := tryLock(held); err != nil {
		t.Fatal(err)
	}
	result := make(chan port.Failure, 1)
	go func() {
		tx, failure := control.Begin(t.Context(), c, true)
		if tx != nil {
			tx.Close()
		}
		result <- failure
	}()
	// The writer has opened the lock and queued behind this holder.
	time.Sleep(300 * time.Millisecond)
	substituteLock(t, home)
	if err := unlock(held); err != nil {
		t.Fatal(err)
	}
	select {
	case failure := <-result:
		if failure != "state.path" {
			t.Fatalf("lock replaced while waiting: %q", failure)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not leave the lock queue")
	}
}

type changedControl struct{ failure port.Failure }
type changedTransaction struct{}

func (c changedControl) Begin(context.Context, port.Candidate, bool) (port.ControlTransaction, port.Failure) {
	if c.failure != "" {
		return nil, c.failure
	}
	return changedTransaction{}, ""
}
func (changedTransaction) Unchanged() bool { return false }
func (changedTransaction) Close()          {}

func TestStoreOperationsRefuseWhenControlChanges(t *testing.T) {
	store, a, _ := diskFixture(t)
	if _, failure := store.Commit(a, ""); failure != "" {
		t.Fatal(failure)
	}
	operations := map[string]func(Store) port.Failure{
		"read":   func(s Store) port.Failure { _, f := s.Read(a); return f },
		"commit": func(s Store) port.Failure { _, f := s.Commit(a, "1"); return f },
		"select": func(s Store) port.Failure { _, f := s.Select(a, "1"); return f },
	}
	for name, run := range operations {
		t.Run(name, func(t *testing.T) {
			changed, refused := store, store
			changed.Control = changedControl{}
			refused.Control = changedControl{failure: "state.busy"}
			if failure := run(changed); failure != "state.path" {
				t.Fatalf("control changed during %s: %q", name, failure)
			}
			if failure := run(refused); failure != "state.busy" {
				t.Fatalf("control refused during %s: %q", name, failure)
			}
		})
	}
}

func TestLocationsInspectRefusesUnsafeAdmissionDirectory(t *testing.T) {
	store, c, _ := diskFixture(t)
	root := filepath.Join(store.StateHome, "itos-container")
	if err := os.MkdirAll(filepath.Join(root, "admission"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "admission"), 0755); err != nil {
		t.Fatal(err)
	}
	directory, failure := (Locations{Store: store}).Inspect(t.Context(), c, root)
	if directory != nil {
		directory.Close()
		t.Fatal("unsafe admission directory was inspected")
	}
	if failure != "state.unsafe" {
		t.Fatalf("inspect: %q, want state.unsafe", failure)
	}
}
