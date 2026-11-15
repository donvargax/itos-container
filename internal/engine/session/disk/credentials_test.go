package disk

import (
	"errors"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func diskCredentialFixture(t *testing.T, values map[string][]byte) f.CredentialFixture {
	t.Helper()
	root := t.TempDir()
	sources := &f.CredentialSourceFixture{}
	for _, kind := range []string{"claude-token", "github-token"} {
		if data, ok := values[kind]; ok {
			path := filepath.Join(t.TempDir(), kind)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal("source setup failed")
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal("source setup failed")
			}
			t.Cleanup(func() { _ = file.Close() })
			info, statErr := file.Stat()
			if statErr != nil {
				t.Fatal("source setup stat failed")
			}
			node, ok := credentialNode(info)
			if !ok {
				t.Fatal("source metadata unavailable")
			}
			sources.Entries = append(sources.Entries, port.CheckedCredential{Kind: kind, ReadPath: "/proc/self/fd/" + strconv.FormatUint(uint64(file.Fd()), 10), Node: node})
		}
	}
	s := Credentials{OS: "linux", root: root, tmpfs: func(string) bool { return true }}
	mapped := func(path string) string {
		return filepath.Join(root, filepath.Base(filepath.Dir(path)), filepath.Base(path))
	}
	return f.CredentialFixture{Fault: func(fault string) port.CredentialStager {
		copy := s
		switch fault {
		case "non-tmpfs":
			copy.tmpfs = func(string) bool { return false }
		case "occupied":
			r := f.CredentialIntent(sources.Kinds())
			if err := os.Mkdir(filepath.Join(root, filepath.Base(r.Credentials.Directory)), 0700); err != nil {
				t.Fatal("collision setup failed")
			}
		case "read":
			copy.read = func(*os.File, []byte) (int, error) { return 0, errors.New("synthetic read error") }
		case "short read":
			copy.read = func(*os.File, []byte) (int, error) { return 1, nil }
		case "write":
			copy.write = func(*os.File, []byte) (int, error) { return 0, errors.New("synthetic write error") }
		case "short write":
			copy.write = func(*os.File, []byte) (int, error) { return 1, nil }
		}
		return copy
	}, Stager: s, Sources: sources, Read: func(path string) []byte { data, _ := os.ReadFile(mapped(path)); return data }, Present: func(path string) bool {
		_, err := os.Lstat(filepath.Join(root, filepath.Base(path)))
		return err == nil
	}}
}
func TestCredentialStagingContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { f.CredentialStagingContract(t, f.MemoryCredentialFixture) })
	t.Run("disk", func(t *testing.T) {
		f.CredentialStagingContract(t, func(v map[string][]byte) f.CredentialFixture { return diskCredentialFixture(t, v) })
	})
}

func TestCredentialRecoveryStore(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		f.CredentialRecoveryStoreContract(t, func(b port.Binding, r port.Record) port.Store {
			_, store, _ := f.Fixture()
			store.Codec.(*f.Codec).Value = b
			store.Records = map[string][]byte{port.Reference(r.Project, r.NameToken()): store.Codec.Encode(r)}
			return store
		})
	})
	t.Run("disk", func(t *testing.T) {
		f.CredentialRecoveryStoreContract(t, func(b port.Binding, r port.Record) port.Store { store, _ := egressStore(b, r); return store })
	})
}
