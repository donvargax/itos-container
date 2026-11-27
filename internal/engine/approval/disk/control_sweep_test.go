package disk

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

type sweepFixtureKind int

const (
	fixtureAdmission sweepFixtureKind = iota
	fixtureIntents
	fixtureSession
)

// swept is a selected managed root that holds one record in each directory
// kind the sweep visits, as a finished run leaves them.
type swept struct {
	control Control
	c       port.Candidate
	root    string
	records map[sweepFixtureKind]string // the record of each directory kind
	dirs    map[sweepFixtureKind]string
}

func sweepIDs() (string, string) { return strings.Repeat("c", 64), strings.Repeat("d", 64) }

// A user namespace can report the host-owned /tmp as the overflow UID.
// Keep its real inode and mode, and present its owner as root to the pin check.
func trustSweepScratch(ops operations) operations {
	lstat := ops.lstat
	ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
		info, err := lstat(root, name)
		if err == nil && root.Name() == "/" && name == "tmp" && info.IsDir() {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid == 65534 {
				stat.Uid = 0
			}
		}
		return info, err
	}
	return ops
}

func sweptTree(t *testing.T) *swept {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("protected storage target is Linux")
	}
	// Every check of a pinned path walks its ancestors. Keep this fixture shallow
	// so a deeply nested harness TMPDIR cannot consume the control call budget.
	base, err := os.MkdirTemp("/tmp", "slice-60-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(base); err != nil {
			t.Error(err)
		}
	})
	home := filepath.Join(base, "account")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	store := Store{StateHome: filepath.Join(base, "selected")}
	ops := trustSweepScratch(store.operations())
	store.ops = &ops
	control := Control{
		Account: &porttest.Account{Home: home}, Store: store,
		Reservations: &porttest.Reservations{RootsByLocation: map[string][]string{}},
	}
	c := port.Candidate{UID: strconv.Itoa(os.Geteuid()), Root: filepath.Join(base, "checkout"), Project: strings.Repeat("a", 64)}
	root := filepath.Join(store.StateHome, "itos-container")
	tx, failure := control.Begin(t.Context(), c, true)
	if tx != nil {
		tx.Close()
	}
	if tx == nil || failure != "" {
		t.Fatalf("Begin = %v, %q; want a transaction", tx, failure)
	}
	s := &swept{control: control, c: c, root: root, records: map[sweepFixtureKind]string{}, dirs: map[sweepFixtureKind]string{}}
	index, id := sweepIDs()
	s.dirs[fixtureAdmission] = filepath.Join(root, "admission")
	s.dirs[fixtureIntents] = filepath.Join(root, "intents", c.Project)
	s.dirs[fixtureSession] = filepath.Join(root, "sessions", index, id)
	s.records[fixtureAdmission] = claimName()
	s.records[fixtureIntents] = strings.Repeat("e", 64) + ".json"
	s.records[fixtureSession] = "record.json"
	for kind, dir := range s.dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, s.records[kind]), []byte("record "+s.records[kind]), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.dirs[fixtureSession], "operation.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *swept) temporary(kind sweepFixtureKind, nonce string) string {
	return filepath.Join(s.dirs[kind], ".intent-"+strings.Repeat(nonce, 32)+".tmp")
}

func (s *swept) lone(t *testing.T, kind sweepFixtureKind, nonce string) string {
	t.Helper()
	path := s.temporary(kind, nonce)
	if err := os.WriteFile(path, []byte("sixteen bytes ok"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (s *swept) link(t *testing.T, kind sweepFixtureKind, nonce string) string {
	t.Helper()
	path := s.temporary(kind, nonce)
	mustLink(t, filepath.Join(s.dirs[kind], s.records[kind]), path)
	return path
}

func (s *swept) begin(t *testing.T, write bool) {
	t.Helper()
	tx, failure := s.control.Begin(t.Context(), s.c, write)
	if tx != nil {
		tx.Close()
	}
	if tx == nil || failure != "" {
		t.Fatalf("Begin(write=%v) = %v, %q; want a transaction", write, tx, failure)
	}
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s remains: %v", path, err)
	}
}

func present(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Errorf("%s was removed: %v", path, err)
	}
}

// identity is what "keeps" means for a record: bytes, inode, mtime and links.
func identity(t *testing.T, path string) [4]any {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	return [4]any{string(data), stat.FieldByName("Ino").Uint(), info.ModTime(), stat.FieldByName("Nlink").Uint()}
}

func TestWriterSweepsLoneAndLinkedTemporariesAndKeepsEveryRecord(t *testing.T) {
	s := sweptTree(t)
	var lone, linked []string
	before := map[string][4]any{}
	for kind := range s.dirs {
		lone = append(lone, s.lone(t, kind, "1"))
		linked = append(linked, s.link(t, kind, "2"))
		path := filepath.Join(s.dirs[kind], s.records[kind])
		before[path] = identity(t, path)
	}
	// Readers never remove anything.
	s.begin(t, false)
	for _, path := range append(lone, linked...) {
		present(t, path)
	}
	s.begin(t, true)
	for _, path := range append(lone, linked...) {
		gone(t, path)
	}
	for path, was := range before {
		now := identity(t, path)
		if now[3] != uint64(1) || !reflect.DeepEqual(was[:3], now[:3]) {
			t.Errorf("%s: %v became %v; want the same bytes, inode and mtime with one link", path, was, now)
		}
	}
}

func TestSweepAlsoCoversIntentSidecars(t *testing.T) {
	s := sweptTree(t)
	sidecar := filepath.Join(s.dirs[fixtureIntents], strings.Repeat("e", 64)+".egress.json")
	if err := os.WriteFile(sidecar, []byte("egress"), 0600); err != nil {
		t.Fatal(err)
	}
	temporary := s.temporary(fixtureIntents, "3")
	mustLink(t, sidecar, temporary)
	s.begin(t, true)
	gone(t, temporary)
	if got := identity(t, sidecar); got[3] != uint64(1) {
		t.Errorf("sidecar links = %v, want 1", got[3])
	}
}

func TestSweepVisitsEveryRootTheTransactionInspected(t *testing.T) {
	s := sweptTree(t)
	// The fixed tree is a root of every transaction.
	home, failure := s.control.Account.AccountHome()
	if failure != "" {
		t.Fatal(failure)
	}
	fixed := filepath.Join(home, ".local", "state", "itos-container", "admission")
	if err := os.Mkdir(fixed, 0700); err != nil {
		t.Fatal(err)
	}
	inFixed := filepath.Join(fixed, ".intent-"+strings.Repeat("4", 32)+".tmp")
	if err := os.WriteFile(inFixed, []byte("sixteen bytes ok"), 0600); err != nil {
		t.Fatal(err)
	}
	// An enrolled location another command selected is a root too.
	enrolled := s.lone(t, fixtureAdmission, "5")
	other := s.control
	other.Store.StateHome = filepath.Join(filepath.Dir(filepath.Dir(s.root)), "another")
	tx, failure := other.Begin(t.Context(), s.c, true)
	if tx != nil {
		tx.Close()
	}
	if failure != "" {
		t.Fatal(failure)
	}
	gone(t, inFixed)
	gone(t, enrolled)
}

func TestSweepKeepsWhatPublishNeverLeaves(t *testing.T) {
	cases := []struct {
		name  string
		kind  sweepFixtureKind
		plant func(t *testing.T, s *swept, path string)
		entry string
	}{
		{"nonce that is not a nonce", fixtureAdmission, lonePlant, ".intent-dead.tmp"},
		{"nonce that is not hex", fixtureAdmission, lonePlant, ".intent-" + strings.Repeat("g", 32) + ".tmp"},
		{"nonce that is upper case", fixtureAdmission, lonePlant, ".intent-" + strings.Repeat("A", 32) + ".tmp"},
		{"nonce that is longer", fixtureAdmission, lonePlant, ".intent-" + strings.Repeat("0", 33) + ".tmp"},
		{"nonce that is shorter", fixtureAdmission, lonePlant, ".intent-" + strings.Repeat("0", 31) + ".tmp"},
		{"suffix that is not tmp", fixtureIntents, lonePlant, ".intent-" + strings.Repeat("0", 32) + ".tmq"},
		{"prefix that is not intent", fixtureIntents, lonePlant, ".intens-" + strings.Repeat("0", 32) + ".tmp"},
		{"approval temporary", fixtureIntents, lonePlant, ".approval-" + strings.Repeat("0", 32) + ".tmp"},
		{"refusals temporary", fixtureSession, lonePlant, ".refusals-" + strings.Repeat("0", 32) + ".tmp"},
		{"mode 0644", fixtureIntents, modePlant(0644), ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"mode 0400", fixtureIntents, modePlant(0400), ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"mode 0700", fixtureIntents, modePlant(0700), ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"setuid", fixtureIntents, modePlant(0600 | os.ModeSetuid), ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"sticky", fixtureIntents, modePlant(0600 | os.ModeSticky), ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"symlink", fixtureAdmission, func(t *testing.T, s *swept, path string) {
			if err := os.Symlink(filepath.Join(s.dirs[fixtureAdmission], s.records[fixtureAdmission]), path); err != nil {
				t.Fatal(err)
			}
		}, ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"directory", fixtureAdmission, func(t *testing.T, _ *swept, path string) {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}, ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"FIFO", fixtureIntents, func(t *testing.T, _ *swept, path string) {
			if err := syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
		}, ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"twin elsewhere", fixtureIntents, func(t *testing.T, s *swept, path string) {
			// A hard link needs the same filesystem. Use the fixture's base
			// outside both managed trees, since the runner's temp directory
			// can be on a different mount from the fixture.
			outside := filepath.Join(filepath.Dir(s.control.Store.StateHome), "outside")
			if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
				t.Fatal(err)
			}
			mustLink(t, outside, path)
		}, ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"twin under another name", fixtureSession, func(t *testing.T, s *swept, path string) {
			linked := filepath.Join(s.dirs[fixtureSession], "linked")
			if err := os.WriteFile(linked, []byte("linked"), 0600); err != nil {
				t.Fatal(err)
			}
			mustLink(t, linked, path)
		}, ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"third link", fixtureAdmission, func(t *testing.T, s *swept, path string) {
			record := filepath.Join(s.dirs[fixtureAdmission], s.records[fixtureAdmission])
			mustLink(t, record, path)
			mustLink(t, record, filepath.Join(filepath.Dir(s.dirs[fixtureAdmission]), "outside"))
		}, ".intent-" + strings.Repeat("1", 32) + ".tmp"},
		{"twin that is another kind's record", fixtureAdmission, func(t *testing.T, s *swept, path string) {
			// "record.json" is a session directory's record, not an admission claim.
			record := filepath.Join(s.dirs[fixtureAdmission], "record.json")
			if err := os.WriteFile(record, []byte("not a claim"), 0600); err != nil {
				t.Fatal(err)
			}
			mustLink(t, record, path)
		}, ".intent-" + strings.Repeat("1", 32) + ".tmp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := sweptTree(t)
			path := filepath.Join(s.dirs[tc.kind], tc.entry)
			tc.plant(t, s, path)
			if tc.name == "mode 0400" || tc.name == "mode 0700" {
				t.Cleanup(func() { _ = os.Chmod(path, 0600) })
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			orphan := s.lone(t, fixtureAdmission, "f")
			s.begin(t, true)
			gone(t, orphan)
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("%s was changed: %v, %v", path, after, err)
			}
		})
	}
}

func lonePlant(t *testing.T, _ *swept, path string) {
	if err := os.WriteFile(path, []byte("sixteen bytes ok"), 0600); err != nil {
		t.Fatal(err)
	}
}

func modePlant(mode os.FileMode) func(*testing.T, *swept, string) {
	return func(t *testing.T, _ *swept, path string) {
		lonePlant(t, nil, path)
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
}

// A directory is swept only while no live publisher can own a temporary in it.
func TestSweepKeepsADirectoryWhoseLockIsNotOurs(t *testing.T) {
	hold := func(t *testing.T, path string) {
		t.Helper()
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = unlock(file); _ = file.Close() })
		if err := tryLock(file); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		kind sweepFixtureKind
		set  func(t *testing.T, s *swept)
	}{
		{"admission collection held", fixtureAdmission, func(t *testing.T, s *swept) { hold(t, s.dirs[fixtureAdmission]) }},
		{"intent directory held", fixtureIntents, func(t *testing.T, s *swept) { hold(t, s.dirs[fixtureIntents]) }},
		{"operation lock held", fixtureSession, func(t *testing.T, s *swept) { hold(t, filepath.Join(s.dirs[fixtureSession], "operation.lock")) }},
		{"operation lock missing", fixtureSession, func(t *testing.T, s *swept) {
			if err := os.Remove(filepath.Join(s.dirs[fixtureSession], "operation.lock")); err != nil {
				t.Fatal(err)
			}
		}},
		{"operation lock is a symlink", fixtureSession, func(t *testing.T, s *swept) {
			path := filepath.Join(s.dirs[fixtureSession], "operation.lock")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(s.dirs[fixtureSession], "record.json"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{"operation lock is not private", fixtureSession, func(t *testing.T, s *swept) {
			if err := os.Chmod(filepath.Join(s.dirs[fixtureSession], "operation.lock"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"operation lock has a second link", fixtureSession, func(t *testing.T, s *swept) {
			mustLink(t, filepath.Join(s.dirs[fixtureSession], "operation.lock"), filepath.Join(s.dirs[fixtureSession], "again"))
		}},
		{"operation lock is not empty", fixtureSession, func(t *testing.T, s *swept) {
			if err := os.WriteFile(filepath.Join(s.dirs[fixtureSession], "operation.lock"), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"operation lock is a directory", fixtureSession, func(t *testing.T, s *swept) {
			path := filepath.Join(s.dirs[fixtureSession], "operation.lock")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sweptTree(t)
			kept := s.lone(t, tc.kind, "1")
			linked := s.link(t, tc.kind, "2")
			// The other directories are not held back.
			var others []string
			for kind := range s.dirs {
				if kind != tc.kind {
					others = append(others, s.lone(t, kind, "3"))
				}
			}
			tc.set(t, s)
			s.begin(t, true)
			present(t, kept)
			present(t, linked)
			for _, path := range others {
				gone(t, path)
			}
		})
	}
}

func TestSweepKeepsDirectoryWhenOperationLockIsReplaced(t *testing.T) {
	s := sweptTree(t)
	kept := s.lone(t, fixtureSession, "1")
	orphan := s.lone(t, fixtureAdmission, "2")
	dir := s.dirs[fixtureSession]
	lock := filepath.Join(dir, "operation.lock")
	newLock := filepath.Join(dir, "replacement")
	if err := os.WriteFile(newLock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ops := s.control.Store.operations()
	lstat := ops.lstat
	checks := 0
	ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
		if root.Name() == dir && name == "operation.lock" {
			checks++
			if checks == 2 {
				if err := os.Rename(newLock, lock); err != nil {
					t.Fatal(err)
				}
			}
		}
		return lstat(root, name)
	}
	s.control.Store.ops = &ops
	s.begin(t, true)
	gone(t, orphan)
	present(t, kept)
	if checks != 2 {
		t.Fatalf("operation lock checks = %d, want 2", checks)
	}
}

// Another user's file is never ours to remove, whatever its name and mode say.
func TestSweepKeepsAFileOfAnotherOwner(t *testing.T) {
	s := sweptTree(t)
	temporary := s.lone(t, fixtureAdmission, "1")
	orphan := s.lone(t, fixtureAdmission, "2")
	ops := s.control.Store.operations()
	lstat := ops.lstat
	ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
		info, err := lstat(root, name)
		if err == nil && name == filepath.Base(temporary) && root.Name() == filepath.Dir(temporary) {
			info.Sys().(*syscall.Stat_t).Uid = uint32(os.Geteuid()) + 1
		}
		return info, err
	}
	s.control.Store.ops = &ops
	s.begin(t, true)
	present(t, temporary)
	gone(t, orphan)
}

// A writer still returns a transaction if its budget ends while sweeping.
func TestSweepStopsWhenTheCallBudgetEnds(t *testing.T) {
	s := sweptTree(t)
	first := s.lone(t, fixtureAdmission, "1")
	next := s.lone(t, fixtureIntents, "2")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := watchTemporary(t, s, next, 1, cancel)
	tx, failure := s.control.Begin(ctx, s.c, true)
	if tx == nil || failure != "" {
		t.Fatalf("Begin = %v, %q; want a transaction", tx, failure)
	}
	tx.Close()
	gone(t, first)
	present(t, next)
	if *calls == 0 {
		t.Fatal("sweep never reached the second temporary")
	}
	s.begin(t, true)
	gone(t, next)
}

func TestSweepKeepsTemporaryWhenBudgetEndsDuringRecheck(t *testing.T) {
	s := sweptTree(t)
	first := s.lone(t, fixtureAdmission, "1")
	next := s.lone(t, fixtureAdmission, "2")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := watchTemporary(t, s, next, 2, cancel)
	tx, failure := s.control.Begin(ctx, s.c, true)
	if tx == nil || failure != "" {
		t.Fatalf("Begin = %v, %q; want a transaction", tx, failure)
	}
	tx.Close()
	gone(t, first)
	present(t, next)
	if *calls < 2 {
		t.Fatal("sweep never reached the temporary's recheck")
	}
}

func watchTemporary(t *testing.T, s *swept, path string, at int, cancel func()) *int {
	t.Helper()
	original := s.control.Store.ops
	ops := s.control.Store.operations()
	checks := 0
	dir, base := filepath.Dir(path), filepath.Base(path)
	ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
		info, err := original.lstat(root, name)
		if root.Name() == dir && name == base {
			checks++
			if checks == at {
				cancel()
			}
		}
		return info, err
	}
	s.control.Store.ops = &ops
	t.Cleanup(func() { s.control.Store.ops = original })
	return &checks
}

// A removal is never allowed to fail the transaction, and a directory that will
// not pin is passed over.
func TestSweepNeverFailsTheTransaction(t *testing.T) {
	s := sweptTree(t)
	kept := s.lone(t, fixtureIntents, "1")
	// A collection another user could write to does not pin: nothing in it is touched.
	if err := os.Chmod(s.dirs[fixtureAdmission], 0755); err != nil {
		t.Fatal(err)
	}
	temporary := s.lone(t, fixtureAdmission, "2")
	// A collection that holds a name no scan expects is not the sweep's to judge.
	if err := os.MkdirAll(filepath.Join(s.root, "sessions", "not-an-index"), 0700); err != nil {
		t.Fatal(err)
	}
	s.begin(t, true)
	present(t, temporary)
	gone(t, kept)
}
