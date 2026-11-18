package disk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestCredentialDirectoryMetadataFailureCleanup(t *testing.T) {
	for _, fault := range []string{"stat", "metadata", "mode", "owner"} {
		t.Run(fault, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			s := fixture.Stager.(Credentials)
			r := fixture.Intent()
			calls := s.operations()
			stat := calls.stat
			reached := false
			calls.stat = func(file *os.File) (fs.FileInfo, error) {
				reached = true
				info, err := stat(file)
				if err != nil || !info.IsDir() {
					t.Fatal("fixture did not reach new directory metadata validation")
				}
				if fault == "stat" {
					return nil, errors.New("synthetic directory metadata failure")
				}
				if fault == "metadata" {
					return credentialStatInfo{info, nil}, nil
				}
				node, _ := credentialNode(info)
				if fault == "mode" {
					node.Mode |= 0040
				} else {
					node.UID++
				}
				raw := struct{ Dev, Ino, Nlink, Uid, Mode uint64 }{node.Device, node.Inode, node.Links, uint64(node.UID), uint64(node.Mode)}
				return credentialStatInfo{info, raw}, nil
			}
			s.ops = &calls
			if failure := s.Stage(t.Context(), r, fixture.Sources, func(port.CredentialStaging) port.Outcome {
				t.Fatal("invalid directory metadata was published")
				return ""
			}); failure != port.CredentialRefused || !reached {
				t.Fatal("directory metadata failure did not retain its refusal", failure)
			}
			path := filepath.Join(s.root, filepath.Base(r.Credentials.Directory))
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("metadata failure stranded an unpublished directory")
			}
			for range 2 {
				if failure := fixture.Stager.Remove(t.Context(), r); failure != "" {
					t.Fatal("intent-only recovery could not confirm cleanup", failure)
				}
			}
			if presence, failure := fixture.Stager.Observe(t.Context(), r); presence != "absent" || failure != "" {
				t.Fatal("recovery did not observe directory absence", failure)
			}
		})
	}
}

func TestCredentialDirectoryMetadataFailurePreservesSubstitution(t *testing.T) {
	for _, replacement := range []string{"directory", "symlink", "file", "missing", "extra entry", "unavailable descriptor"} {
		t.Run(replacement, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			s := fixture.Stager.(Credentials)
			r := fixture.Intent()
			path := filepath.Join(s.root, filepath.Base(r.Credentials.Directory))
			retained := filepath.Join(s.root, "retained")
			calls := s.operations()
			reached := false
			calls.stat = func(file *os.File) (fs.FileInfo, error) {
				reached = true
				var err error
				switch replacement {
				case "unavailable descriptor":
					err = file.Close()
				case "extra entry":
					err = os.WriteFile(filepath.Join(path, "unexpected"), []byte("retain"), 0400)
				default:
					if err = os.Rename(path, retained); err != nil {
						t.Fatal(err)
					}
					switch replacement {
					case "directory":
						err = os.Mkdir(path, 0700)
					case "symlink":
						err = os.Symlink(retained, path)
					case "file":
						err = os.WriteFile(path, []byte("replacement"), 0400)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				return nil, errors.New("synthetic directory metadata failure")
			}
			s.ops = &calls
			if failure := s.Stage(t.Context(), r, fixture.Sources, func(port.CredentialStaging) port.Outcome {
				t.Fatal("failed directory metadata was published")
				return ""
			}); failure != port.CredentialRefused || !reached {
				t.Fatal("substitution changed the metadata refusal", failure)
			}
			if replacement == "missing" {
				if failure := fixture.Stager.Remove(t.Context(), r); failure != "" {
					t.Fatal("missing unpublished directory blocked recovery", failure)
				}
			} else {
				if _, err := os.Lstat(path); err != nil {
					t.Fatal("cleanup removed a substituted or unconfirmed directory entry")
				}
				if failure := fixture.Stager.Remove(t.Context(), r); failure != port.CredentialRefused {
					t.Fatal("intent-only recovery adopted unconfirmed staging", failure)
				}
			}
			switch replacement {
			case "directory", "symlink", "file", "missing":
				if _, err := os.Stat(retained); err != nil {
					t.Fatal("cleanup removed the descriptor's renamed directory")
				}
			case "extra entry":
				if data, err := os.ReadFile(filepath.Join(path, "unexpected")); err != nil || string(data) != "retain" {
					t.Fatal("cleanup changed an unexpected entry")
				}
			}
		})
	}
}
