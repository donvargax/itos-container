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
	read := func(session string) []byte {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(directory), session, "egress-refusals.jsonl"))
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
	derived := sp.RefusalReport{Records: 1, Path: path}
	sf.RefusalLogContract(t, sf.RefusalLogFixture{
		SessionID: id,
		Log:       log,
		Read:      read,
		Fault:     func(name string) { fault = name },
		Report:    func(sp.RefusalReport) {},
	}, []byte(sf.ProxyStdout), []byte(lines[1]), []byte(lines[0]+replacement), []byte(replacement), derived)

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
	if !slices.Equal(names, []string{"controller.lock", "egress-refusals.json", "egress-refusals.jsonl", "operation.lock", "record.json"}) {
		t.Fatal("the session directory holds a temporary or lost a file", names)
	}
	if failure := held.Validate(); failure != "" {
		t.Fatal("the held session no longer validates beside its log", failure)
	}
	// The committed report reads back through the same port on a real state
	// root: a clean copy reports its stored count, and one whose source skipped
	// sequences reports the exact inclusive ranges, ready's omission included.
	for _, tc := range []struct {
		name    string
		input   string
		stored  string
		state   string
		records uint64
		gaps    []sp.RefusalGap
	}{
		{"clean", sf.ProxyStdout, lines[1], "stored", 1, nil},
		{"initial gap", lines[1], lines[1], "gapped", 1, []sp.RefusalGap{{First: 1, Last: 1}}},
		{"middle gap", lines[0] + lines[1] + strings.Replace(lines[1], `"seq":2`, `"seq":5`, 1) + strings.Replace(lines[1], `"seq":2`, `"seq":7`, 1), lines[1] + strings.Replace(lines[1], `"seq":2`, `"seq":5`, 1) + strings.Replace(lines[1], `"seq":2`, `"seq":7`, 1), "gapped", 3, []sp.RefusalGap{{First: 3, Last: 4}, {First: 6, Last: 6}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy, failure := log.Begin(ctx, id)
			if failure != "" {
				t.Fatal(failure)
			}
			if n, err := copy.Write([]byte(tc.input)); n != len(tc.input) || err != nil {
				t.Fatal(n, err)
			}
			report, failure := copy.Commit()
			copy.Close()
			if failure != "" || report.Outcome != (map[bool]sp.Outcome{true: sp.RefusalLogGap, false: ""}[len(tc.gaps) > 0]) || !slices.Equal(report.Gaps, tc.gaps) || report.Records != tc.records {
				t.Fatalf("commit = %#v, %s", report, failure)
			}
			evidence, failure := log.Read(ctx, id)
			if failure != "" || evidence.Path != path || evidence.State != tc.state || evidence.Records == nil || *evidence.Records != tc.records || !slices.Equal(evidence.Missing, tc.gaps) {
				t.Fatalf("evidence = %#v, %s", evidence, failure)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != tc.stored {
				t.Fatalf("published bytes changed: %q", data)
			}
		})
	}
}
