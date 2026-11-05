package disk

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

type frozenRecords struct {
	records
	a, b port.Candidate
}

func (c frozenRecords) FrozenRecord(f port.Frozen) (port.Record, port.Failure) {
	candidate := c.a
	if f.Settings == c.b.Canonical {
		candidate = c.b
	}
	if f.Settings != candidate.Canonical || f.Generation == "" {
		return port.Record{}, "state.invalid"
	}
	return port.Record{Candidate: candidate, Generation: f.Generation}, ""
}
func snapshotFixture(t *testing.T) (Snapshots, port.Frozen) {
	t.Helper()
	store, a, b := diskFixture(t)
	codec := frozenRecords{a: a, b: b}
	store.Records = codec
	return Snapshots{Store: store, Records: codec}, port.Frozen{Project: a.Project, Generation: "1", Fingerprint: a.Fingerprint, Settings: a.Canonical}
}
func TestSnapshotRefusesMalformedFrozenValueAndStateLocation(t *testing.T) {
	s, f := snapshotFixture(t)
	f.Settings = "invalid"
	if h, err := s.Publish(f); err != "snapshot.invalid" || h.Frozen != (port.Frozen{}) {
		t.Fatal(h, err)
	}
	s, f = snapshotFixture(t)
	s.Store.StateHome = "relative"
	if h, err := s.Publish(f); err != "snapshot.unsafe" || h.Frozen != (port.Frozen{}) {
		t.Fatal(h, err)
	}
}
func TestSnapshotWriteFailuresNeverReturnUsableInput(t *testing.T) {
	for _, kind := range []string{"nonce", "create", "chmod", "short write", "write", "file fsync", "close", "rename", "parent fsync", "ancestor fsync", "substituted parent"} {
		t.Run(kind, func(t *testing.T) {
			s, f := snapshotFixture(t)
			ops := s.Store.operations()
			fault := errors.New("publication failed")
			switch kind {
			case "nonce":
				ops.nonce = func([]byte) (int, error) { return 0, fault }
			case "create":
				ops.create = func(*os.Root, string) (*os.File, error) { return nil, fault }
			case "chmod":
				ops.chmod = func(*os.File, os.FileMode) error { return fault }
			case "short write":
				ops.write = func(file *os.File, b []byte) (int, error) { return file.Write(b[:len(b)-1]) }
			case "write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, fault }
			case "file fsync":
				ops.syncFile = func(*os.File) error { return fault }
			case "close":
				ops.closeFile = func(file *os.File) error { return errors.Join(file.Close(), fault) }
			case "rename":
				ops.rename = func(*os.Root, string, string) error { return fault }
			case "parent fsync":
				ops.syncParent = func(*os.File) error { return fault }
			case "ancestor fsync":
				ops.syncDir = func(*os.File) error { return fault }
			case "substituted parent":
				ops.syncFile = func(file *os.File) error {
					err := file.Sync()
					path := filepath.Join(s.Store.StateHome, "itos-container", "snapshots", f.Project)
					if e := os.Rename(path, path+"-moved"); e != nil {
						t.Fatal(e)
					}
					if e := os.Mkdir(path, 0700); e != nil {
						t.Fatal(e)
					}
					return err
				}
			}
			s.Store.ops = &ops
			h, err := s.Publish(f)
			want := port.Failure("snapshot.write")
			if kind == "substituted parent" {
				want = "snapshot.unsafe"
			}
			if err != want || h.Frozen != (port.Frozen{}) {
				t.Fatalf("usable or wrong refusal %#v %s", h, err)
			}
			// A failed parent sync may leave a complete record, but no torn record.
			path := filepath.Join(s.Store.StateHome, "itos-container", "snapshots", f.Project, "1.json")
			if data, e := os.ReadFile(path); e == nil {
				c, _ := s.Records.FrozenRecord(f)
				if record, failure := s.Records.Decode(c.Candidate, data); failure != "" || record != c {
					t.Fatal("partial final record", record, failure)
				}
			} else if !os.IsNotExist(e) {
				t.Fatal(e)
			}
		})
	}
}
func TestSnapshotReuseRevalidatesTheFileAndDurability(t *testing.T) {
	for _, kind := range []string{"open", "stat", "unsafe file", "read", "different bytes", "file fsync", "parent fsync", "ancestor open", "ancestor fsync", "name substitution"} {
		t.Run(kind, func(t *testing.T) {
			s, f := snapshotFixture(t)
			h, err := s.Publish(f)
			if err != "" {
				t.Fatal(err)
			}
			ops := s.Store.operations()
			fault := errors.New("reuse failed")
			// Only the second open is durability's reopen; the first validates the record.
			opens := 0
			open := ops.openRecord
			ops.openRecord = func(r *os.Root, n string, flags int, mode os.FileMode) (*os.File, error) {
				opens++
				if kind == "open" && opens == 2 {
					return nil, fault
				}
				file, e := open(r, n, flags, mode)
				if e == nil && opens == 2 && kind == "unsafe file" {
					e = file.Chmod(0644)
				}
				return file, e
			}
			stat := ops.statFile
			ops.statFile = func(file *os.File) (os.FileInfo, error) {
				if kind == "stat" && opens == 2 {
					return nil, fault
				}
				return stat(file)
			}
			read := ops.readAll
			ops.readAll = func(r io.Reader) ([]byte, error) {
				if opens == 2 {
					if kind == "read" {
						return nil, fault
					}
					if kind == "different bytes" {
						return []byte("conflict"), nil
					}
				}
				return read(r)
			}
			switch kind {
			case "file fsync":
				ops.syncFile = func(*os.File) error { return fault }
			case "parent fsync":
				ops.syncParent = func(*os.File) error { return fault }
			case "ancestor open":
				open := ops.openDir
				ops.openDir = func(r *os.Root, n string) (*os.File, error) {
					if opens == 2 {
						return nil, fault
					}
					return open(r, n)
				}
			case "ancestor fsync":
				ops.syncDir = func(*os.File) error { return fault }
			case "name substitution":
				ops.syncParent = func(file *os.File) error {
					err := file.Sync()
					if e := os.Rename(h.Reference, h.Reference+"-old"); e != nil {
						t.Fatal(e)
					}
					if e := os.WriteFile(h.Reference, []byte("replacement"), 0600); e != nil {
						t.Fatal(e)
					}
					return err
				}
			}
			s.Store.ops = &ops
			got, failure := s.Publish(f)
			if failure == "" || got.Frozen != (port.Frozen{}) {
				t.Fatalf("reuse returned usable input after %s: %#v %s", kind, got, failure)
			}
		})
	}
}
