package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	sessioncodec "github.com/donvargax/itos-container/internal/engine/session/codec"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

// contractSession hands Begin a registered session in place of the name the
// shared contract passes, which no registry holds.
type contractSession struct {
	log sp.RefusalLog
	id  string
}

func (l contractSession) Begin(ctx context.Context, session string) (sp.RefusalCopy, sp.Outcome) {
	if session == "session" {
		session = l.id
	}
	return l.log.Begin(ctx, session)
}

// The two concrete adapters meet only here: session's RefusalLog over approval's
// protected registry, on a real state root.
func TestRefusalLogDiskContract(t *testing.T) {
	store, d, storage := sessionStoreFixture(t)
	protected := approvaldisk.Registry{Store: storage.Store}
	registry := sessiondisk.Registry{Store: store, Storage: protected, Codec: store.Codec.(sessioncodec.JSON), Identity: canonical.JSON{}}
	id, token := strings.Repeat("b", 64), strings.Repeat("c", 64)
	ctx := context.Background()
	lease, failure := registry.Register(ctx, d, id, token)
	if failure != "" {
		t.Fatal(failure)
	}
	defer lease.Close()
	intent, failure := store.New(ctx, d, token)
	if failure != "" {
		t.Fatal(failure)
	}
	intent.Close()
	// The controller holds the session open for writing while it copies.
	held, failure := registry.Open(ctx, id, false)
	if failure != "" {
		t.Fatal(failure)
	}
	defer held.Close()

	var fault string
	broken := errors.New("fixture refusal log fault")
	faulted := protected
	faulted.IO = approvaldisk.SnapshotIO{
		Write: func(file *os.File, data []byte) (int, error) {
			if fault == "write" {
				return 0, broken
			}
			return file.Write(data)
		},
		FileSync: func(file *os.File) error {
			if fault == "commit" {
				return broken
			}
			return file.Sync()
		},
	}
	log := sessiondisk.RefusalLog{Store: store, Storage: faulted}
	if copy, failure := log.Begin(ctx, "session"); copy != nil || failure != sp.Invalid {
		t.Fatal("the adapter accepted the contract's own session name", failure)
	}
	directory := filepath.Join(storage.Store.StateHome, "itos-container", "sessions", publicIndexKey(store.Input.Root, held.Intent().Binding().UID), id)
	if _, err := os.Stat(filepath.Join(directory, "record.json")); err != nil {
		t.Fatal("the log must land beside the session's record", err)
	}
	path := filepath.Join(directory, "egress-refusals.jsonl")
	read := func(string) []byte {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	lines := strings.SplitAfter(sf.ProxyStdout, "\n")
	replacement := strings.Replace(lines[1], "not-listed", "port", 1)
	sf.RefusalLogContract(t, sf.RefusalLogFixture{
		Log:    contractSession{log, id},
		Read:   read,
		Fault:  func(name string) { fault = name },
		Report: func(sp.RefusalReport) {},
	}, []byte(sf.ProxyStdout), []byte(lines[1]), []byte(lines[0]+replacement), []byte(replacement), sp.RefusalReport{})

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatal("the stored log is not a private regular file", info.Mode())
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{"controller.lock", "egress-refusals.jsonl", "operation.lock", "record.json"}) {
		t.Fatal("the session directory holds a temporary or lost a file", names)
	}
	if failure := held.Validate(); failure != "" {
		t.Fatal("the held session no longer validates beside its log", failure)
	}
}
