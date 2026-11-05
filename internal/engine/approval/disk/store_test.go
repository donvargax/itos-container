package disk

import (
	"encoding/json"
	"errors"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The disk contract receives an independent fixture codec. CLI/cmd contracts
// separately bind the real canonical codec to the real disk store.
type records struct{}

func (records) Encode(r port.Record) []byte { data, _ := json.Marshal(r); return data }
func (records) Decode(c port.Candidate, data []byte) (port.Record, port.Failure) {
	var r port.Record
	if json.Unmarshal(data, &r) != nil || r.Project != c.Project {
		return port.Record{}, "state.invalid"
	}
	return r, ""
}
func diskFixture(t *testing.T) (Store, port.Candidate, port.Candidate) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("protected storage target is Linux")
	}
	dir := t.TempDir()
	store := Store{StateHome: dir, Records: records{}}
	a := port.Candidate{UID: strconv.Itoa(os.Geteuid()), Root: filepath.Join(dir, "checkout"), Project: strings.Repeat("a", 64), Canonical: "A", Fingerprint: "a"}
	b := a
	b.Canonical = "B"
	b.Fingerprint = "b"
	return store, a, b
}
func TestDiskStoreContract(t *testing.T) {
	porttest.StoreContract(t, func() (port.Store, port.Candidate, port.Candidate) { s, a, b := diskFixture(t); return s, a, b })
}
func TestPublicationFaults(t *testing.T) {
	for _, kind := range []string{"nonce", "create", "chmod", "short write", "write", "file sync", "close", "rename", "parent sync", "parent replaced", "after sync replaced", "after publication replaced"} {
		t.Run(kind, func(t *testing.T) {
			store, a, b := diskFixture(t)
			if _, failure := store.Commit(a, ""); failure != "" {
				t.Fatal(failure)
			}
			ops := store.operations()
			fault := errors.New("fixture publication failure")
			state := filepath.Join(store.StateHome, "itos-container")
			moved := state + "-old"
			replace := func() {
				if err := os.Rename(state, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(state, 0700); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "nonce":
				ops.nonce = func([]byte) (int, error) { return 0, fault }
			case "create":
				ops.create = func(*os.Root, string) (*os.File, error) { return nil, fault }
			case "chmod":
				ops.chmod = func(*os.File, os.FileMode) error { return fault }
			case "short write":
				ops.write = func(_ *os.File, b []byte) (int, error) { return len(b) - 1, nil }
			case "write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, fault }
			case "file sync":
				ops.syncFile = func(*os.File) error { return fault }
			case "close":
				ops.closeFile = func(f *os.File) error { return errors.Join(f.Close(), fault) }
			case "rename":
				ops.rename = func(*os.Root, string, string) error { return fault }
			case "parent sync":
				ops.syncParent = func(*os.File) error { return fault }
			case "parent replaced":
				ops.beforePublish = replace
			case "after sync replaced":
				ops.syncFile = func(f *os.File) error { err := f.Sync(); replace(); return err }
			case "after publication replaced":
				ops.syncParent = func(f *os.File) error { err := f.Sync(); replace(); return err }
			}
			store.ops = &ops
			record, failure := store.Commit(b, "1")
			want := port.Failure("state.write")
			if strings.Contains(kind, "replaced") {
				want = "state.path"
			}
			if failure != want || record.Generation != "" {
				t.Fatalf("failure = %s, record %#v", failure, record)
			}
			path := filepath.Join(state, "approvals", a.Project, "current.json")
			if strings.Contains(kind, "replaced") {
				path = filepath.Join(moved, "approvals", a.Project, "current.json")
				entries, err := os.ReadDir(state)
				if err != nil || len(entries) != 0 {
					t.Fatal("replacement target changed", err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, failure := (records{}).Decode(a, data)
			if failure != "" {
				t.Fatal("partial record")
			}
			expected := "A"
			if kind == "parent sync" || kind == "after publication replaced" {
				expected = "B"
			}
			if got.Canonical != expected {
				t.Fatal(got)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil || len(entries) != 1 {
				t.Fatal("temporary publication leaked", entries, err)
			}
		})
	}
}
func TestStoreLocationsAndOverflow(t *testing.T) {
	s, a, b := diskFixture(t)
	for _, location := range []string{"relative", a.Root, filepath.Join(a.Root, "state")} {
		s.StateHome = location
		_, failure := s.Read(a)
		want := port.Failure("state.path")
		if location == "relative" {
			want = "state.location"
		}
		if failure != want {
			t.Fatal(location, failure)
		}
	}
	s.StateHome = ""
	s.Home = "relative"
	if _, f := s.Read(a); f != "state.location" {
		t.Fatal(f)
	}
	s.Home = t.TempDir()
	if _, f := s.Commit(a, ""); f != "" {
		t.Fatal(f)
	}
	path := filepath.Join(s.Home, ".local/state/itos-container/approvals", a.Project, "current.json")
	if err := os.WriteFile(path, (records{}).Encode(port.Record{Candidate: a, Generation: "18446744073709551615"}), 0600); err != nil {
		t.Fatal(err)
	}
	if _, f := s.Commit(b, "18446744073709551615"); f != "state.invalid" {
		t.Fatal(f)
	}
}

type ownedInfo struct {
	os.FileInfo
	system any
}

func (i ownedInfo) Sys() any { return i.system }
func TestRefuseForeignOwnership(t *testing.T) {
	s, a, _ := diskFixture(t)
	if _, f := s.Commit(a, ""); f != "" {
		t.Fatal(f)
	}
	for _, path := range []string{filepath.Join(s.StateHome, "itos-container"), filepath.Join(s.StateHome, "itos-container/approvals", a.Project, "current.json")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		copyStat := reflect.New(stat.Type())
		copyStat.Elem().Set(stat)
		copyStat.Elem().FieldByName("Uid").SetUint(uint64(os.Geteuid() + 1))
		foreign := ownedInfo{info, copyStat.Interface()}
		failure := recordSafety(foreign, uint64(os.Geteuid()))
		if info.IsDir() {
			failure = directory(foreign, uint64(os.Geteuid()), true)
		}
		if failure != "state.unsafe" {
			t.Fatal(failure)
		}
	}
}
