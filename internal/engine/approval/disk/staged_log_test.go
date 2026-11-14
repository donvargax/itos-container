package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

// stagedSession registers one session through r and returns its ID and directory.
func stagedSession(t *testing.T, r Registry, c port.Candidate) (string, string) {
	t.Helper()
	id := strings.Repeat("a", 64)
	lease, failure := r.Register(context.Background(), c, id, []byte("record"))
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(lease.Close)
	sessions, failure := r.Store.locationAt(c, "sessions")
	if failure != "" {
		t.Fatal(failure)
	}
	return id, filepath.Join(sessions, id)
}

// stagedTemporary reports a name of the staged log's temporary: ".refusals-",
// the 32 lowercase hex digits of a 128-bit nonce and ".tmp".
func stagedTemporary(name string) bool {
	return len(name) == 46 && strings.HasPrefix(name, ".refusals-") && strings.HasSuffix(name, ".tmp") && strings.Trim(name[10:42], "0123456789abcdef") == ""
}

// sessionNames lists a session directory, sorted; a missing one holds nothing.
func sessionNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func stagedTemporaries(t *testing.T, dir string) []string {
	t.Helper()
	return slices.DeleteFunc(sessionNames(t, dir), func(name string) bool { return !stagedTemporary(name) })
}

// storedLog reads the published log, nil when the name does not exist.
func storedLog(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, stagedLogName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte{}, data...)
}

// handles records every directory and file a store opens through its
// operations, so a test can show that a released staged log kept none of them:
// neither its file nor any directory of its pin.
type handles struct {
	roots []*os.Root
	files []*os.File
}

// trackHandles wraps the operations as they stand, so install it after any fault.
func trackHandles(ops *operations) *handles {
	h := &handles{}
	openRoot, openChild, openDir, create := ops.openRoot, ops.openChild, ops.openDir, ops.create
	ops.openRoot = func(name string) (*os.Root, error) {
		root, err := openRoot(name)
		if err == nil {
			h.roots = append(h.roots, root)
		}
		return root, err
	}
	ops.openChild = func(parent *os.Root, name string) (*os.Root, error) {
		root, err := openChild(parent, name)
		if err == nil {
			h.roots = append(h.roots, root)
		}
		return root, err
	}
	ops.openDir = func(root *os.Root, name string) (*os.File, error) {
		file, err := openDir(root, name)
		if err == nil {
			h.files = append(h.files, file)
		}
		return file, err
	}
	ops.create = func(root *os.Root, name string) (*os.File, error) {
		file, err := create(root, name)
		if err == nil {
			h.files = append(h.files, file)
		}
		return file, err
	}
	return h
}

// open counts the recorded handles that have not been closed.
func (h *handles) open() int {
	n := 0
	for _, root := range h.roots {
		if _, err := root.Stat("."); !errors.Is(err, os.ErrClosed) {
			n++
		}
	}
	for _, file := range h.files {
		if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
			n++
		}
	}
	return n
}

// publishLog stages, writes and publishes one clean log.
func publishLog(t *testing.T, r Registry, c port.Candidate, id, data string) {
	t.Helper()
	staged, failure := r.Stage(context.Background(), c, id)
	if failure != "" || staged == nil {
		t.Fatalf("stage = %v, %s", staged, failure)
	}
	if failure = staged.Write([]byte(data)); failure != "" {
		t.Fatal(failure)
	}
	if failure = staged.Publish(); failure != "" {
		t.Fatal(failure)
	}
}

func TestSessionLogsContract(t *testing.T) {
	for _, backend := range []string{"memory", "disk"} {
		t.Run(backend, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			if backend == "memory" {
				files := &porttest.Registry{}
				porttest.SessionLogsContract(t, porttest.SessionLogsFixture{Storage: files, Logs: files, Candidate: c,
					Read:   func(id string) []byte { data, _ := files.SessionLog(c.Project, id); return data },
					Staged: func(id string) int { _, staged := files.SessionLog(c.Project, id); return staged },
					Fault:  func(name string) { files.Fault = name }})
				return
			}
			var fault string
			broken := errors.New("fixture staged log fault")
			r := Registry{Store: s, IO: SnapshotIO{
				Write: func(file *os.File, data []byte) (int, error) {
					if fault == "write" {
						return 0, broken
					}
					return file.Write(data)
				},
				FileSync: func(file *os.File) error {
					if fault == "file fsync" {
						return broken
					}
					return file.Sync()
				},
				Rename: func(root *os.Root, from, to string) error {
					if fault == "atomic publication" {
						return broken
					}
					return root.Rename(from, to)
				},
				ParentSync: func(file *os.File) error {
					if fault == "parent fsync" {
						return broken
					}
					return file.Sync()
				},
			}}
			sessions, failure := s.locationAt(c, "sessions")
			if failure != "" {
				t.Fatal(failure)
			}
			porttest.SessionLogsContract(t, porttest.SessionLogsFixture{Storage: r, Logs: r, Candidate: c,
				Read:   func(id string) []byte { return storedLog(t, filepath.Join(sessions, id)) },
				Staged: func(id string) int { return len(stagedTemporaries(t, filepath.Join(sessions, id))) },
				Fault:  func(name string) { fault = name }})
		})
	}
}

func TestStagedLogFaults(t *testing.T) {
	for _, kind := range []string{"none", "directory open", "directory sync", "nonce", "create", "chmod", "short write", "write", "short write then close", "write then close",
		"file sync", "close", "rename", "parent sync", "replaced before rename", "replaced after rename"} {
		t.Run(kind, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			id, dir := stagedSession(t, Registry{Store: s}, c)
			publishLog(t, Registry{Store: s}, c, id, "earlier\n")
			ops := s.operations()
			fault := errors.New("fixture staged log fault")
			state := filepath.Join(s.StateHome, "itos-container")
			moved := state + "-old"
			replace := func() {
				if err := os.Rename(state, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(state, 0700); err != nil {
					t.Fatal(err)
				}
			}
			writes := 0
			write := ops.write
			ops.write = func(file *os.File, data []byte) (int, error) { writes++; return write(file, data) }
			stage, wrote, published := port.Failure(""), port.Failure(""), port.Failure("state.write")
			visible := "earlier\n"
			switch kind {
			case "none":
				published, visible = "", "next\n"
			case "directory open":
				// The pin opens its own directory once; every later open is a sync's.
				stage = "state.write"
				opened, openDir := 0, ops.openDir
				ops.openDir = func(root *os.Root, name string) (*os.File, error) {
					if opened++; opened > 1 {
						return nil, fault
					}
					return openDir(root, name)
				}
			case "directory sync":
				stage = "state.write"
				ops.syncDir = func(*os.File) error { return fault }
			case "nonce":
				stage = "state.write"
				ops.nonce = func([]byte) (int, error) { return 0, fault }
			case "create":
				stage = "state.write"
				ops.create = func(*os.Root, string) (*os.File, error) { return nil, fault }
			case "chmod":
				stage = "state.write"
				ops.chmod = func(*os.File, os.FileMode) error { return fault }
			case "short write", "short write then close":
				wrote = "state.write"
				ops.write = func(file *os.File, data []byte) (int, error) { writes++; return write(file, data[:len(data)-1]) }
			case "write", "write then close":
				wrote = "state.write"
				ops.write = func(*os.File, []byte) (int, error) { writes++; return 0, fault }
			case "file sync":
				ops.syncFile = func(*os.File) error { return fault }
			case "close":
				ops.closeFile = func(file *os.File) error { return errors.Join(file.Close(), fault) }
			case "rename":
				ops.rename = func(*os.Root, string, string) error { return fault }
			case "parent sync":
				visible = "next\n"
				ops.syncParent = func(*os.File) error { return fault }
			case "replaced before rename":
				published = "state.path"
				ops.syncFile = func(file *os.File) error { err := file.Sync(); replace(); return err }
			case "replaced after rename":
				published, visible = "state.path", "next\n"
				ops.syncParent = func(file *os.File) error { err := file.Sync(); replace(); return err }
			}
			opened := trackHandles(&ops)
			s.ops = &ops
			staged, failure := (Registry{Store: s}).Stage(context.Background(), c, id)
			if failure != stage || (staged == nil) != (stage != "") {
				t.Fatalf("stage = %v, %s; want %q", staged, failure, stage)
			}
			if staged != nil {
				// The file, the session directory and one root for each level of the pin.
				if held := opened.open(); held != len(strings.Split(dir, string(filepath.Separator)))+2 {
					t.Fatal("a staged log holds its file and its pin", held)
				}
				if names := stagedTemporaries(t, dir); len(names) != 1 {
					t.Fatal("a staged log has exactly one temporary", names)
				}
				if failure = staged.Write([]byte("next\n")); failure != wrote {
					t.Fatalf("write = %s, want %q", failure, wrote)
				}
				if wrote != "" {
					if failure = staged.Write([]byte("more\n")); failure != "state.write" || writes != 1 {
						t.Fatalf("poisoned write = %s after %d file writes", failure, writes)
					}
				}
				if strings.HasSuffix(kind, "then close") {
					staged.Close()
					published = "state.conflict"
				}
				if failure = staged.Publish(); failure != published {
					t.Fatalf("publish = %s, want %q", failure, published)
				}
				if failure = staged.Publish(); failure != "state.conflict" {
					t.Fatal("second publish", failure)
				}
				if failure = staged.Write([]byte("late\n")); failure != "state.write" {
					t.Fatal("write after release", failure)
				}
				staged.Close()
			}
			if held := opened.open(); held != 0 || len(opened.files) == 0 {
				t.Fatalf("the pin or the file was not released: %d of %d handles are open", held, len(opened.roots)+len(opened.files))
			}
			if strings.HasPrefix(kind, "replaced") {
				dir = strings.Replace(dir, state, moved, 1)
				if entries, err := os.ReadDir(state); err != nil || len(entries) != 0 {
					t.Fatal("replacement target changed", entries, err)
				}
			}
			if got := string(storedLog(t, dir)); got != visible {
				t.Fatalf("visible log = %q, want %q", got, visible)
			}
			if names := sessionNames(t, dir); !slices.Equal(names, []string{"controller.lock", stagedLogName, "operation.lock", "record.json"}) {
				t.Fatal("temporary left, or a session file lost", names)
			}
		})
	}
}

func TestStagedLogUnsafeTargets(t *testing.T) {
	for _, tc := range []struct {
		name           string
		stage, publish port.Failure
	}{
		{"safe", "", ""},
		{"group bit", "state.unsafe", ""},
		{"other bit", "state.unsafe", ""},
		{"group bit on the lookup directory", "state.unsafe", ""},
		{"group bit on the sessions directory", "state.unsafe", ""},
		{"group bit on the managed root", "state.unsafe", ""},
		{"relative state home", "state.location", ""},
		{"symlinked session", "state.path", ""},
		{"symlink at the name", "", ""},
		{"directory at the name", "", "state.write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			r := Registry{Store: s}
			id, dir := stagedSession(t, r, c)
			final := filepath.Join(dir, stagedLogName)
			target := filepath.Join(s.StateHome, "target")
			inspected := dir
			switch tc.name {
			case "group bit":
				if err := os.Chmod(dir, 0710); err != nil {
					t.Fatal(err)
				}
			case "other bit":
				if err := os.Chmod(dir, 0701); err != nil {
					t.Fatal(err)
				}
			case "group bit on the lookup directory", "group bit on the sessions directory", "group bit on the managed root":
				// A readable, unwritable group bit passes an unmanaged ancestor's
				// check, so this holds each of the four managed levels to 0700.
				level := map[string]string{"lookup": filepath.Dir(dir), "sessions": filepath.Dir(filepath.Dir(dir)), "managed": filepath.Join(s.StateHome, "itos-container")}[strings.Fields(tc.name)[4]]
				if err := os.Chmod(level, 0750); err != nil {
					t.Fatal(err)
				}
			case "relative state home":
				r.Store.StateHome = "relative"
			case "symlinked session":
				inspected = dir + "-moved"
				if err := os.Rename(dir, inspected); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(inspected, dir); err != nil {
					t.Fatal(err)
				}
			case "symlink at the name":
				if err := os.WriteFile(target, []byte("target"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, final); err != nil {
					t.Fatal(err)
				}
			case "directory at the name":
				if err := os.Mkdir(final, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ops := r.Store.operations()
			opened := trackHandles(&ops)
			r.Store.ops = &ops
			before := sessionNames(t, inspected)
			staged, failure := r.Stage(context.Background(), c, id)
			if failure != tc.stage || (staged == nil) != (tc.stage != "") {
				t.Fatalf("stage = %v, %s; want %q", staged, failure, tc.stage)
			}
			if staged == nil {
				if after := sessionNames(t, inspected); !slices.Equal(after, before) || opened.open() != 0 {
					t.Fatal("a refused stage created something or kept its pin", after, opened.open())
				}
				return
			}
			if failure = staged.Write([]byte("next\n")); failure != "" {
				t.Fatal(failure)
			}
			if failure = staged.Publish(); failure != tc.publish {
				t.Fatalf("publish = %s, want %q", failure, tc.publish)
			}
			if names := stagedTemporaries(t, dir); len(names) != 0 || opened.open() != 0 {
				t.Fatal("temporary or pin left", names, opened.open())
			}
			info, err := os.Lstat(final)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "directory at the name" {
				if !info.IsDir() {
					t.Fatal("the directory at the name was replaced", info.Mode())
				}
				return
			}
			if !info.Mode().IsRegular() || string(storedLog(t, dir)) != "next\n" {
				t.Fatal("the name does not hold the published log", info.Mode())
			}
			if tc.name == "symlink at the name" {
				if data, err := os.ReadFile(target); err != nil || string(data) != "target" {
					t.Fatalf("the symlink's target was written through: %q %v", data, err)
				}
			}
		})
	}
}
