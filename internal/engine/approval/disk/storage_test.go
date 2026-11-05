package disk

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
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
	s, c, _ := diskFixture(t)
	storage := Storage{Store: s}
	ctx := context.Background()
	first, f := storage.Open(ctx, c, "intents", true)
	if f != "" {
		t.Fatal(f)
	}
	defer first.Close()
	timed, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if second, f := storage.Open(timed, c, "intents", false); second != nil || f != "state.unavailable" {
		t.Fatal(second, f)
	}
}
func TestProtectedReadAndPublicationFailures(t *testing.T) {
	for _, fault := range []string{"read", "record stat", "open record", "oversize", "nonce", "create", "chmod", "write", "short write", "file sync", "close", "rename", "parent sync", "ancestor sync", "exclusive conflict"} {
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
