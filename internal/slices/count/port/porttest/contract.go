package porttest

import (
	"bytes"

	"github.com/donvargax/itos-container/internal/slices/count/port"
)

// Fixture supplies logical input paths for the shared Files contract.
// The unreadable fixture is a directory on disk, so privileged runners agree.
type Fixture struct{ Empty, Data, Missing, Unreadable string }

// Reporter is the testing output used by the contract. A recording fake lets
// its tests verify that faulty adapters really trigger each assertion.
type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// FilesContract runs the same port assertions against fakes and adapters.
// Its factory pattern follows itos-cc's FilesContract (ADR-0047/0051);
// the assertions here describe this project's byte-reading port.
func FilesContract(t Reporter, open func() (port.Files, Fixture)) {
	t.Helper()
	{
		files, paths := open()
		for _, c := range []struct {
			path string
			want []byte
		}{{paths.Empty, []byte{}}, {paths.Data, []byte("one\x00two\n")}} {
			first, failure := files.Read(c.path)
			if failure != nil || !bytes.Equal(first, c.want) {
				t.Errorf("Read(%q) = %q, %v; want %q, nil", c.path, first, failure, c.want)
			}
			second, failure := files.Read(c.path)
			if failure != nil || !bytes.Equal(second, c.want) {
				t.Errorf("repeat Read(%q) = %q, %v", c.path, second, failure)
			}
			// Mutate every byte: partially overlapping storage is also a breach.
			for i := range first {
				first[i] ^= 0xff
			}
			if !bytes.Equal(second, c.want) {
				t.Errorf("Read(%q) shares returned storage", c.path)
			}
			again, failure := files.Read(c.path)
			if failure != nil || !bytes.Equal(again, c.want) {
				t.Errorf("Read(%q) retained caller mutation: %q, %v", c.path, again, failure)
			}
		}
	}
	{
		files, paths := open()
		for _, path := range []string{paths.Missing, paths.Unreadable} {
			data, failure := files.Read(path)
			if data != nil {
				t.Errorf("Read(%q) returned failure data %q", path, data)
			}
			if path == paths.Missing {
				missing, ok := failure.(*port.Missing)
				if !ok || missing.Path != path {
					t.Errorf("Read(%q) = %T %v; want Missing carrying path", path, failure, failure)
				}
			} else {
				unreadable, ok := failure.(*port.Unreadable)
				if !ok || unreadable.Path != path {
					t.Errorf("Read(%q) = %T %v; want Unreadable carrying path", path, failure, failure)
				}
			}
		}
	}
}

// ContractFiles creates the in-memory inputs promised by FilesContract.
func ContractFiles() (Files, Fixture) {
	paths := Fixture{Empty: "empty", Data: "data", Missing: "missing", Unreadable: "directory"}
	return Files{Data: map[string][]byte{paths.Empty: {}, paths.Data: []byte("one\x00two\n")}, Failures: map[string]port.ReadFailure{paths.Unreadable: &port.Unreadable{Path: paths.Unreadable}}}, paths
}
