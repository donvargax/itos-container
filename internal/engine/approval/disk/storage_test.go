package disk

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

func TestProtectedStorageContract(t *testing.T) {
	t.Run("disk", func(t *testing.T) {
		porttest.StorageContract(t, func() (port.Storage, port.Candidate) { s, c, _ := diskFixture(t); return Storage{Store: s}, c })
	})
	t.Run("memory", func(t *testing.T) {
		porttest.StorageContract(t, func() (port.Storage, port.Candidate) {
			return &porttest.Storage{}, port.Candidate{Project: strings.Repeat("a", 64)}
		})
	})
}
func TestStorageLockRespectsCallerDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, c, _ := diskFixture(t)
		storage := Storage{Store: s}
		ctx := context.Background()
		first, f := storage.Open(ctx, c, "intents", true)
		if f != "" {
			t.Fatal(f)
		}
		defer first.Close()
		// Virtual time puts cancellation between lock polls, so slow filesystem
		// setup cannot expire the context before the waiter reaches its select.
		const budget = 15 * time.Millisecond
		timed, cancel := context.WithTimeout(ctx, budget)
		defer cancel()
		started := time.Now()
		if second, f := storage.Open(timed, c, "intents", false); second != nil || f != "state.busy" {
			t.Fatal(second, f)
		}
		if elapsed := time.Since(started); elapsed != budget || !errors.Is(timed.Err(), context.DeadlineExceeded) {
			t.Fatalf("lock wait = %v, %v; want the caller's %v deadline", elapsed, timed.Err(), budget)
		}
		if !first.Unchanged() {
			t.Fatal("canceled waiter invalidated the lock holder")
		}
	})
}

func TestStorageCancellationBeforeLockAcquisition(t *testing.T) {
	s, c, _ := diskFixture(t)
	storage := Storage{Store: s}
	d, failure := storage.Open(context.Background(), c, "intents", true)
	if failure != "" {
		t.Fatal(failure)
	}
	d.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ops := s.operations()
	openDir := ops.openDir
	var lockFile *os.File
	ops.openDir = func(root *os.Root, name string) (*os.File, error) {
		file, err := openDir(root, name)
		if err == nil {
			lockFile = file
			cancel()
		}
		return file, err
	}
	s.ops = &ops
	if opened, f := (Storage{Store: s}).Open(ctx, c, "intents", false); opened != nil || f != "state.busy" {
		t.Fatalf("canceled lock acquisition = %v, %s", opened, f)
	}
	if lockFile == nil {
		t.Fatal("cancellation did not reach lock acquisition")
	}
	if _, err := lockFile.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("canceled lock acquisition leaked its directory handle", err)
	}
	// A later caller can still acquire the same protected directory.
	retry, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	d, failure = storage.Open(retry, c, "intents", false)
	if failure != "" || d == nil {
		t.Fatalf("open after canceled acquisition = %v, %s", d, failure)
	}
	d.Close()
}

func TestProtectedReadRejectsChangedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		want port.Failure
	}{
		{"canceled context", "state.path"},
		{"lookup failure", "state.unavailable"},
		{"unsafe descriptor", "state.unsafe"},
		{"replaced descriptor", "state.path"},
		{"lookup failure after read", "state.path"},
		{"replacement after read", "state.path"},
		{"unsafe metadata after read", "state.path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var descriptor os.FileInfo
			descriptorReached := false
			storage := Storage{Store: s, Stat: func(file *os.File) (os.FileInfo, error) {
				if filepath.Base(file.Name()) == "1.json" && descriptor != nil {
					descriptorReached = true
					return descriptor, nil
				}
				return file.Stat()
			}}
			opened, failure := storage.Open(ctx, c, "intents", true)
			if failure != "" {
				t.Fatal(failure)
			}
			d := opened.(*directoryStore)
			defer d.Close()
			if failure = d.Publish("1.json", nil, []byte("original")); failure != "" {
				t.Fatal(failure)
			}
			// Independent metadata represents either a safe replacement inode or
			// an unsafe file; the original protected record remains untouched.
			other := filepath.Join(t.TempDir(), "other")
			if err := os.WriteFile(other, []byte("outside"), 0600); err != nil {
				t.Fatal(err)
			}
			replacement, err := os.Stat(other)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Chmod(other, 0644); err != nil {
				t.Fatal(err)
			}
			unsafe, err := os.Stat(other)
			if err != nil {
				t.Fatal(err)
			}
			switch tc.name {
			case "canceled context":
				cancel()
			case "unsafe descriptor":
				descriptor = unsafe
			case "replaced descriptor":
				descriptor = replacement
			}
			lstat := d.pin.ops.lstat
			lookups := 0
			d.pin.ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
				if name == "1.json" {
					lookups++
					if tc.name == "lookup failure" || lookups == 2 && tc.name == "lookup failure after read" {
						return nil, os.ErrPermission
					}
					if lookups == 2 && tc.name == "replacement after read" {
						return replacement, nil
					}
					if lookups == 2 && tc.name == "unsafe metadata after read" {
						// Keep the inode unchanged so the final ownership/mode
						// validation, not the replacement check, must refuse it.
						if err := root.Chmod(name, 0644); err != nil {
							t.Fatal(err)
						}
					}
				}
				return lstat(root, name)
			}
			if data, f := d.Read("1.json"); data != nil || f != tc.want {
				t.Fatalf("unsafe read = %q, %s; want no data, %s", data, f, tc.want)
			}
			if descriptor != nil && !descriptorReached {
				t.Fatal("read did not validate opened record metadata")
			}
			if strings.HasSuffix(tc.name, "after read") && lookups != 2 {
				t.Fatal("read did not reach final record validation", lookups)
			}
			data, err := os.ReadFile(filepath.Join(d.pin.root().Name(), "1.json"))
			if err != nil || string(data) != "original" {
				t.Fatal("failed read changed the protected record", err)
			}
		})
	}
}

func TestProtectedReadAndPublicationFailures(t *testing.T) {
	for _, fault := range []string{"read", "record stat", "open record", "oversize", "nonce", "create", "chmod", "write", "short write", "file sync", "close", "rename", "parent sync", "ancestor open", "ancestor sync", "exclusive conflict"} {
		t.Run(fault, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			storage := Storage{Store: s}
			ctx := context.Background()
			opened, f := storage.Open(ctx, c, "intents", true)
			if f != "" {
				t.Fatal(f)
			}
			d := opened.(*directoryStore)
			defer d.Close()
			if f = d.Publish("1.json", nil, []byte("before")); f != "" {
				t.Fatal(f)
			}
			ops := &d.pin.ops
			broken := errors.New("injected file operation failure")
			switch fault {
			case "read":
				ops.readAll = func(io.Reader) ([]byte, error) { return nil, broken }
			case "record stat":
				ops.statFile = func(*os.File) (os.FileInfo, error) { return nil, broken }
			case "open record":
				ops.openRecord = func(*os.Root, string, int, os.FileMode) (*os.File, error) { return nil, broken }
			case "oversize":
				ops.readAll = func(io.Reader) ([]byte, error) { return make([]byte, 1024*1024+1), nil }
			case "nonce":
				ops.nonce = func([]byte) (int, error) { return 0, broken }
			case "create":
				ops.create = func(*os.Root, string) (*os.File, error) { return nil, broken }
			case "chmod":
				ops.chmod = func(*os.File, os.FileMode) error { return broken }
			case "write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, broken }
			case "short write":
				ops.write = func(_ *os.File, b []byte) (int, error) { return len(b) - 1, nil }
			case "file sync":
				ops.syncFile = func(*os.File) error { return broken }
			case "close":
				ops.closeFile = func(f *os.File) error { return errors.Join(f.Close(), broken) }
			case "rename":
				ops.rename = func(*os.Root, string, string) error { return broken }
			case "parent sync":
				ops.syncParent = func(*os.File) error { return broken }
			case "ancestor open":
				ops.openDir = func(*os.Root, string) (*os.File, error) { return nil, broken }
			case "ancestor sync":
				ops.syncDir = func(*os.File) error { return broken }
			case "exclusive conflict":
				d.exclusive = func(*os.Root, string, string) error { return os.ErrExist }
			}
			name, previous := "1.json", []byte("before")
			if fault == "exclusive conflict" {
				name, previous = "2.json", nil
			}
			if f = d.Publish(name, previous, []byte("after")); f == "" {
				t.Fatal("failed operation reported successful publication")
			}
			if fault == "ancestor open" {
				if f != "state.write" {
					t.Fatal("ancestor open failure was not a write failure", f)
				}
				data, failure := d.Read("1.json")
				if failure != "" || string(data) != "before" {
					t.Fatal("failed ancestor open changed the preceding record", failure)
				}
			}
		})
	}
}
func TestStorageRejectsScopeAndBadBytes(t *testing.T) {
	s, c, _ := diskFixture(t)
	storage := Storage{Store: s}
	if _, f := storage.Open(context.Background(), c, "approvals", true); f != "state.invalid" {
		t.Fatal(f)
	}
	if _, f := storage.Open(context.Background(), c, "snapshots", true); f != "state.invalid" {
		t.Fatal(f)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, f := storage.Open(canceled, c, "intents", true); f != "state.unavailable" {
		t.Fatal(f)
	}
	d, f := storage.Open(context.Background(), c, "intents", true)
	if f != "" {
		t.Fatal(f)
	}
	defer d.Close()
	if f = d.Publish("1.json", nil, nil); f != "state.invalid" {
		t.Fatal(f)
	}
	if f = d.Sync("1.json", nil); f != "state.invalid" {
		t.Fatal(f)
	}
	if f = d.Publish("1.json", []byte("missing"), []byte("next")); f != "state.conflict" {
		t.Fatal(f)
	}
}

func TestProtectedPublicationRevalidatesBeforeAndAfterPublication(t *testing.T) {
	for _, tc := range []struct {
		name    string
		want    port.Failure
		visible string
	}{
		{"path replaced after file sync", "state.path", "before"},
		{"record lookup fails after file sync", "state.unavailable", "before"},
		{"record changes after file sync", "state.conflict", "competing"},
		{"empty record appears after file sync", "state.conflict", ""},
		{"exclusive publication fails", "state.write", ""},
		{"temporary link disappears", "state.write", "after"},
		{"path replaced after parent sync", "state.path", "after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			opened, failure := (Storage{Store: s}).Open(context.Background(), c, "intents", true)
			if failure != "" {
				t.Fatal(failure)
			}
			d := opened.(*directoryStore)
			defer d.Close()
			previous := []byte("before")
			if tc.name == "empty record appears after file sync" || tc.name == "exclusive publication fails" || tc.name == "temporary link disappears" {
				previous = nil
			} else if failure = d.Publish("1.json", nil, previous); failure != "" {
				t.Fatal(failure)
			}
			state := filepath.Join(s.StateHome, "itos-container")
			moved := state + "-old"
			reached := false
			replace := func() {
				if err := os.Rename(state, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(state, 0700); err != nil {
					t.Fatal(err)
				}
			}
			switch tc.name {
			case "exclusive publication fails", "temporary link disappears":
				d.exclusive = func(root *os.Root, temporary, name string) error {
					reached = true
					if tc.name == "exclusive publication fails" {
						return os.ErrPermission
					}
					if err := root.Link(temporary, name); err != nil {
						t.Fatal(err)
					}
					// The complete final record survives, but the temporary name
					// disappears before the publisher can confirm its removal.
					return root.Remove(temporary)
				}
			case "path replaced after parent sync":
				d.pin.ops.syncParent = func(file *os.File) error {
					if err := file.Sync(); err != nil {
						return err
					}
					reached = true
					replace()
					return nil
				}
			default:
				d.pin.ops.syncFile = func(file *os.File) error {
					if err := file.Sync(); err != nil {
						return err
					}
					reached = true
					switch tc.name {
					case "path replaced after file sync":
						replace()
					case "record lookup fails after file sync":
						lstat := d.pin.ops.lstat
						d.pin.ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
							if name == "1.json" {
								return nil, os.ErrPermission
							}
							return lstat(root, name)
						}
					default:
						if err := os.WriteFile(filepath.Join(d.pin.root().Name(), "1.json"), []byte(tc.visible), 0600); err != nil {
							t.Fatal(err)
						}
					}
					return nil
				}
			}
			if failure = d.Publish("1.json", previous, []byte("after")); failure != tc.want || !reached {
				t.Fatalf("publication = %s, want %s (fault reached %v)", failure, tc.want, reached)
			}
			recordRoot := state
			if strings.HasPrefix(tc.name, "path replaced") {
				recordRoot = moved
				entries, err := os.ReadDir(state)
				if err != nil || len(entries) != 0 {
					t.Fatal("publication changed the replacement directory", entries, err)
				}
			}
			dir := filepath.Join(recordRoot, "intents", c.Project)
			data, err := os.ReadFile(filepath.Join(dir, "1.json"))
			wantEntries := 1
			if tc.name == "exclusive publication fails" {
				wantEntries = 0
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed publication left a final record", err)
				}
			} else if err != nil || string(data) != tc.visible {
				t.Fatalf("visible record = %q, %v; want %q", data, err, tc.visible)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != wantEntries {
				t.Fatal("publication leaked temporary evidence", entries, err)
			}
		})
	}
}

func TestProtectedSyncRetainsRecordWhenReadFails(t *testing.T) {
	s, c, _ := diskFixture(t)
	opened, failure := (Storage{Store: s}).Open(context.Background(), c, "intents", true)
	if failure != "" {
		t.Fatal(failure)
	}
	d := opened.(*directoryStore)
	defer d.Close()
	data := []byte("complete ownership")
	if failure = d.Publish("1.json", nil, data); failure != "" {
		t.Fatal(failure)
	}
	readReached := false
	d.pin.ops.readAll = func(io.Reader) ([]byte, error) {
		readReached = true
		return nil, os.ErrPermission
	}
	d.pin.ops.syncFile = func(*os.File) error {
		t.Fatal("unreadable ownership reached durability confirmation")
		return nil
	}
	if failure = d.Sync("1.json", data); failure != "state.unavailable" || !readReached {
		t.Fatalf("sync = %s (read reached %v)", failure, readReached)
	}
	visible, err := os.ReadFile(filepath.Join(d.pin.root().Name(), "1.json"))
	if err != nil || string(visible) != string(data) {
		t.Fatal("failed sync changed the complete ownership record", err)
	}
}
