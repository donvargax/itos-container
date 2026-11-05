package disk

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func TestPinRevalidationFailures(t *testing.T) {
	for _, kind := range []string{"root open", "child open", "child stat"} {
		t.Run(kind, func(t *testing.T) {
			s, a, _ := diskFixture(t)
			prior, failure := s.Commit(a, "")
			if failure != "" {
				t.Fatal(failure)
			}
			ops := s.operations()
			real := ops
			revalidating := false
			fault := errors.New("fixture revalidation failure")
			ops.lock = func(f *os.File, exclusive bool) error {
				revalidating = true
				return real.lock(f, exclusive)
			}
			ops.openRoot = func(path string) (*os.Root, error) {
				if revalidating && kind == "root open" {
					return nil, fault
				}
				return real.openRoot(path)
			}
			ops.openChild = func(r *os.Root, name string) (*os.Root, error) {
				if revalidating && kind == "child open" {
					return nil, fault
				}
				return real.openChild(r, name)
			}
			ops.statRoot = func(r *os.Root, name string) (os.FileInfo, error) {
				if revalidating && kind == "child stat" {
					return nil, fault
				}
				return real.statRoot(r, name)
			}
			s.ops = &ops
			if got, failure := s.Read(a); failure != "state.path" || got != (port.Record{}) {
				t.Fatalf("read = %#v, %q", got, failure)
			}
			s.ops = nil
			if got, failure := s.Read(a); failure != "" || got != prior {
				t.Fatalf("revalidation failure changed prior approval: %#v, %q", got, failure)
			}
		})
	}
}

func TestRecordReadFailures(t *testing.T) {
	cases := []struct {
		name string
		want port.Failure
	}{
		{"lookup", "state.unavailable"}, {"symlink", "state.path"}, {"permissions", "state.unsafe"},
		{"open", "state.path"}, {"stat", "state.path"}, {"read", "state.unavailable"},
		{"path recheck", "state.path"}, {"restat", "state.unavailable"}, {"permission recheck", "state.unsafe"},
		{"invalid bytes", "state.invalid"},
	}
	for _, tc := range cases {
		for _, action := range []string{"read", "commit", "select"} {
			t.Run(tc.name+"/"+action, func(t *testing.T) {
				s, a, b := diskFixture(t)
				if _, failure := s.Commit(a, ""); failure != "" {
					t.Fatal(failure)
				}
				path := filepath.Join(s.StateHome, "itos-container", "approvals", a.Project, "current.json")
				switch tc.name {
				case "symlink":
					if err := os.Rename(path, path+"-target"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(path+"-target", path); err != nil {
						t.Fatal(err)
					}
				case "permissions":
					if err := os.Chmod(path, 0644); err != nil {
						t.Fatal(err)
					}
				case "invalid bytes":
					if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				ops := s.operations()
				real := ops
				fault := errors.New("fixture record failure")
				ops.lstat = func(r *os.Root, name string) (os.FileInfo, error) {
					if name == "current.json" && tc.name == "lookup" {
						return nil, fault
					}
					return real.lstat(r, name)
				}
				ops.openRecord = func(r *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
					if tc.name == "open" {
						return nil, fault
					}
					return real.openRecord(r, name, flags, mode)
				}
				stats := 0
				ops.statFile = func(f *os.File) (os.FileInfo, error) {
					stats++
					if stats == 1 && tc.name == "stat" || stats == 2 && tc.name == "restat" {
						return nil, fault
					}
					return real.statFile(f)
				}
				read := false
				ops.readAll = func(r io.Reader) ([]byte, error) {
					if tc.name == "read" {
						return nil, fault
					}
					data, err := real.readAll(r)
					read = true
					if tc.name == "permission recheck" {
						if err := os.Chmod(path, 0644); err != nil {
							t.Fatal(err)
						}
					}
					return data, err
				}
				ops.openRoot = func(path string) (*os.Root, error) {
					if read && tc.name == "path recheck" {
						return nil, fault
					}
					return real.openRoot(path)
				}
				s.ops = &ops
				var failure port.Failure
				switch action {
				case "read", "commit":
					var record port.Record
					if action == "read" {
						record, failure = s.Read(a)
					} else {
						record, failure = s.Commit(b, "1")
					}
					if record != (port.Record{}) {
						t.Fatalf("failed %s returned approval: %#v", action, record)
					}
				case "select":
					var frozen port.Frozen
					frozen, failure = s.Select(a, "1")
					if frozen != (port.Frozen{}) {
						t.Fatalf("failed selection returned frozen input: %#v", frozen)
					}
				}
				if failure != tc.want {
					t.Fatalf("%s failure = %q; want %q", action, failure, tc.want)
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("failed %s altered existing record: %v", action, err)
				}
			})
		}
	}
}

type invalidRecords struct {
	records
	data []byte
}

func (r invalidRecords) Encode(port.Record) []byte { return r.data }

func TestInvalidEncodingCannotReplaceApproval(t *testing.T) {
	for _, data := range [][]byte{nil, bytes.Repeat([]byte("x"), 1024*1024+1)} {
		s, a, b := diskFixture(t)
		prior, failure := s.Commit(a, "")
		if failure != "" {
			t.Fatal(failure)
		}
		s.Records = invalidRecords{data: data}
		if got, failure := s.Commit(b, "1"); failure != "state.invalid" || got != (port.Record{}) {
			t.Fatalf("invalid encoding published: %#v, %q", got, failure)
		}
		if got, failure := s.Read(a); failure != "" || got != prior {
			t.Fatalf("invalid encoding changed prior approval: %#v, %q", got, failure)
		}
	}
}

func TestEmptyProjectDirectoryIsNotApproval(t *testing.T) {
	s, a, _ := diskFixture(t)
	path := filepath.Join(s.StateHome, "itos-container", "approvals", a.Project)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if frozen, failure := s.Select(a, "1"); failure != "missing" || frozen != (port.Frozen{}) {
		t.Fatalf("empty directory yielded frozen input: %#v, %q", frozen, failure)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("read-only selection wrote project state: %v, %v", entries, err)
	}
}
