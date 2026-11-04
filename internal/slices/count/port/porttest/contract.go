package porttest

import (
	"bytes"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/count/port"
)

// Fixture supplies logical input paths for the shared Files contract.
// The unreadable fixture is a directory on disk, so privileged runners agree.
type Fixture struct{ Empty, Data, Missing, Unreadable string }

// FilesContract runs the same port assertions against fakes and adapters.
// Its factory pattern follows itos-cc's FilesContract (ADR-0047/0051);
// the assertions here describe this project's byte-reading port.
func FilesContract(t *testing.T, open func(*testing.T) (port.Files, Fixture)) {
	t.Helper()
	t.Run("successful reads preserve bytes and independent storage", func(t *testing.T) {
		files, paths := open(t)
		for _, c := range []struct {
			path string
			want []byte
		}{{paths.Empty, []byte{}}, {paths.Data, []byte("one\x00two\n")}} {
			first, failure := files.Read(c.path)
			if failure != nil || !bytes.Equal(first, c.want) {
				t.Fatalf("Read(%q) = %q, %v; want %q, nil", c.path, first, failure, c.want)
			}
			second, failure := files.Read(c.path)
			if failure != nil || !bytes.Equal(second, c.want) {
				t.Fatalf("repeat Read(%q) = %q, %v", c.path, second, failure)
			}
			if len(first) > 0 {
				first[0] = 'X'
			}
			if !bytes.Equal(second, c.want) {
				t.Fatalf("Read(%q) shares returned storage", c.path)
			}
			again, failure := files.Read(c.path)
			if failure != nil || !bytes.Equal(again, c.want) {
				t.Fatalf("Read(%q) retained caller mutation: %q, %v", c.path, again, failure)
			}
		}
	})
	t.Run("failures carry requested paths and no data", func(t *testing.T) {
		files, paths := open(t)
		for _, path := range []string{paths.Missing, paths.Unreadable} {
			data, failure := files.Read(path)
			if data != nil {
				t.Fatalf("Read(%q) returned failure data %q", path, data)
			}
			if path == paths.Missing {
				missing, ok := failure.(*port.Missing)
				if !ok || missing.Path != path {
					t.Fatalf("Read(%q) = %T %v; want Missing carrying path", path, failure, failure)
				}
			} else {
				unreadable, ok := failure.(*port.Unreadable)
				if !ok || unreadable.Path != path {
					t.Fatalf("Read(%q) = %T %v; want Unreadable carrying path", path, failure, failure)
				}
			}
		}
	})
}

// ContractFiles creates the in-memory inputs promised by FilesContract.
func ContractFiles() (Files, Fixture) {
	paths := Fixture{Empty: "empty", Data: "data", Missing: "missing", Unreadable: "directory"}
	return Files{Data: map[string][]byte{paths.Empty: {}, paths.Data: []byte("one\x00two\n")}, Failures: map[string]port.ReadFailure{paths.Unreadable: &port.Unreadable{Path: paths.Unreadable}}}, paths
}
