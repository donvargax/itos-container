package disk

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func TestInvalidStorageIdentities(t *testing.T) {
	for _, project := range []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("G", 64), "../" + strings.Repeat("a", 61)} {
		t.Run("project-"+project, func(t *testing.T) {
			s, a, _ := diskFixture(t)
			a.Project = project
			assertStoreRefusal(t, s, a, "state.invalid")
		})
	}
	for _, uid := range []string{"", "-1", "host", "4294967296"} {
		t.Run("uid-"+uid, func(t *testing.T) {
			s, a, _ := diskFixture(t)
			a.UID = uid
			assertStoreRefusal(t, s, a, "state.unsafe")
		})
	}
}

func assertStoreRefusal(t *testing.T, s Store, a port.Candidate, want port.Failure) {
	t.Helper()
	if got, failure := s.Read(a); failure != want || got != (port.Record{}) {
		t.Fatalf("read = %#v, %q; want empty, %q", got, failure, want)
	}
	if got, failure := s.Commit(a, ""); failure != want || got != (port.Record{}) {
		t.Fatalf("commit = %#v, %q; want empty, %q", got, failure, want)
	}
	if got, failure := s.Select(a, "1"); failure != want || got != (port.Frozen{}) {
		t.Fatalf("select = %#v, %q; want empty, %q", got, failure, want)
	}
}

// The real adapter and real directory handles run throughout. Only the selected
// OS operation fails; no fake store supplies the refusal or cleanup verdict.
func TestPinInitializationFailures(t *testing.T) {
	cases := []struct {
		name string
		want port.Failure
	}{
		{"root open", "state.unavailable"}, {"lookup", "state.unavailable"},
		{"mkdir", "state.write"}, {"parent open", "state.write"}, {"parent sync", "state.write"},
		{"private parent open", "state.write"}, {"private open", "state.path"},
		{"private stat", "state.path"}, {"private chmod", "state.write"}, {"private recheck", "state.path"},
		{"child open", "state.path"}, {"child stat", "state.path"},
		{"directory open", "state.write"}, {"directory sync", "state.write"}, {"directory restat", "state.write"},
		{"lock open", "state.unavailable"}, {"lock", "state.unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, a, _ := diskFixture(t)
			ops := s.operations()
			real := ops
			managed := filepath.Join(s.StateHome, "itos-container")
			project := filepath.Join(managed, "approvals", a.Project)
			fault := errors.New("fixture initialization failure")
			var roots []*os.Root
			var files []*os.File
			injected := false
			fail := func(kind string) bool {
				if tc.name == kind {
					injected = true
					return true
				}
				return false
			}
			ops.openRoot = func(path string) (*os.Root, error) {
				if fail("root open") {
					return nil, fault
				}
				r, err := real.openRoot(path)
				roots = append(roots, r)
				return r, err
			}
			lookups := 0
			ops.lstat = func(r *os.Root, name string) (os.FileInfo, error) {
				if filepath.Join(r.Name(), name) == managed {
					lookups++
					if fail("lookup") || lookups == 3 && fail("private recheck") {
						return nil, fault
					}
				}
				return real.lstat(r, name)
			}
			ops.mkdir = func(r *os.Root, name string, mode os.FileMode) error {
				if fail("mkdir") {
					return fault
				}
				return real.mkdir(r, name, mode)
			}
			parentOpens, projectOpens := 0, 0
			ops.openDir = func(r *os.Root, name string) (*os.File, error) {
				path := filepath.Join(r.Name(), name)
				if path == s.StateHome {
					parentOpens++
					if parentOpens == 1 && fail("parent open") || parentOpens == 2 && fail("private parent open") {
						return nil, fault
					}
				}
				if path == project {
					projectOpens++
					if projectOpens == 2 && fail("lock open") {
						return nil, fault
					}
				}
				if path == managed && fail("directory open") {
					return nil, fault
				}
				f, err := real.openDir(r, name)
				files = append(files, f)
				return f, err
			}
			ops.syncDir = func(f *os.File) error {
				if filepath.Clean(f.Name()) == s.StateHome && fail("parent sync") || filepath.Clean(f.Name()) == managed && fail("directory sync") {
					return fault
				}
				return real.syncDir(f)
			}
			ops.openPrivate = func(fd uintptr, name string) (int, error) {
				if fail("private open") {
					return -1, fault
				}
				return real.openPrivate(fd, name)
			}
			ops.statFile = func(f *os.File) (os.FileInfo, error) {
				if fail("private stat") {
					return nil, fault
				}
				return real.statFile(f)
			}
			ops.chmodPrivate = func(fd uintptr) error {
				if fail("private chmod") {
					return fault
				}
				return real.chmodPrivate(fd)
			}
			ops.openChild = func(r *os.Root, name string) (*os.Root, error) {
				if filepath.Join(r.Name(), name) == managed && fail("child open") {
					return nil, fault
				}
				next, err := real.openChild(r, name)
				roots = append(roots, next)
				return next, err
			}
			stats := 0
			ops.statRoot = func(r *os.Root, name string) (os.FileInfo, error) {
				if r.Name() == managed {
					stats++
					if stats == 1 && fail("child stat") || stats == 2 && fail("directory restat") {
						return nil, fault
					}
				}
				return real.statRoot(r, name)
			}
			ops.lock = func(f *os.File, exclusive bool) error {
				if fail("lock") {
					return fault
				}
				return real.lock(f, exclusive)
			}
			s.ops = &ops
			if got, failure := s.Commit(a, ""); failure != tc.want || got != (port.Record{}) {
				t.Fatalf("commit = %#v, %q; want empty, %q", got, failure, tc.want)
			}
			if !injected {
				t.Fatal("fixture did not reach the selected filesystem fault")
			}
			for _, r := range roots {
				if r != nil {
					if _, err := r.Stat("."); !errors.Is(err, os.ErrClosed) {
						t.Errorf("root handle leaked: %s, %v", r.Name(), err)
					}
				}
			}
			for _, f := range files {
				if f != nil {
					if _, err := f.Stat(); !errors.Is(err, os.ErrClosed) {
						t.Errorf("directory handle leaked: %s, %v", f.Name(), err)
					}
				}
			}
			if _, err := os.Lstat(filepath.Join(project, "current.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed initialization published an approval: %v", err)
			}
			s.ops = nil
			if _, failure := s.Commit(a, ""); failure != "" {
				t.Fatalf("retry after failed initialization: %s", failure)
			}
		})
	}
}

func TestUnsafeStateDirectories(t *testing.T) {
	for _, kind := range []string{"symlink", "regular file", "managed permissions", "ancestor permissions"} {
		t.Run(kind, func(t *testing.T) {
			s, a, _ := diskFixture(t)
			path := filepath.Join(s.StateHome, "itos-container")
			want := port.Failure("state.unsafe")
			switch kind {
			case "symlink":
				if err := os.Symlink(t.TempDir(), path); err != nil {
					t.Fatal(err)
				}
				want = "state.path"
			case "regular file":
				if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
			case "managed permissions", "ancestor permissions":
				if kind == "ancestor permissions" {
					path = s.StateHome
				} else if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0777); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			assertStoreRefusal(t, s, a, want)
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("unsafe entry was repaired or replaced: %v, %v", after, err)
			}
		})
	}
}

func TestDirectoryRequiresKnownTrustedOwnership(t *testing.T) {
	s, _, _ := diskFixture(t)
	info, err := os.Stat(s.StateHome)
	if err != nil {
		t.Fatal(err)
	}
	uid := uint64(os.Geteuid())
	for _, system := range []any{nil, 1, struct{}{}, struct{ Uid uint32 }{uint32(uid)}} {
		unknown := ownedInfo{FileInfo: info, system: system}
		if failure := directory(unknown, uid, false); failure != "state.unsafe" {
			t.Fatalf("accepted unknown ownership %T: %q", system, failure)
		}
	}
	// Root is trusted as an ancestor; a different ordinary UID is not.
	foreign := ownedInfo{FileInfo: info, system: &struct{ Uid, Nlink uint64 }{uid + 1, 1}}
	if failure := directory(foreign, uid, false); failure != "state.unsafe" {
		t.Fatalf("accepted foreign ancestor: %q", failure)
	}
}
