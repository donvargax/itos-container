package disk

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestCredentialFileMetadataFailureCleanup(t *testing.T) {
	for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
		for _, failedKind := range kinds.List() {
			for _, fault := range []string{"stat", "metadata", "mode", "owner", "links"} {
				t.Run(strings.Join(kinds.List(), "+")+"/"+failedKind+"/"+fault, func(t *testing.T) {
					values := map[string][]byte{}
					for _, kind := range kinds.List() {
						values[kind] = []byte("synthetic\n")
					}
					fixture := diskCredentialFixture(t, values)
					s := fixture.Stager.(Credentials)
					r := fixture.Intent()
					calls := s.operations()
					stat := calls.stat
					reached := false
					calls.stat = func(file *os.File) (fs.FileInfo, error) {
						info, err := stat(file)
						if filepath.Base(file.Name()) != failedKind || err != nil {
							return info, err
						}
						reached = true
						if info.Size() != 0 {
							t.Fatal("copy preceded metadata validation")
						}
						if fault == "stat" {
							return nil, errors.New("synthetic metadata failure")
						}
						if fault == "metadata" {
							return credentialStatInfo{info, nil}, nil
						}
						node, _ := credentialNode(info)
						switch fault {
						case "mode":
							node.Mode |= 0040
						case "owner":
							node.UID++
						case "links":
							node.Links++
						}
						raw := struct{ Dev, Ino, Nlink, Uid, Mode uint64 }{node.Device, node.Inode, node.Links, uint64(node.UID), uint64(node.Mode)}
						return credentialStatInfo{info, raw}, nil
					}
					s.ops = &calls
					failure := s.Stage(t.Context(), r, fixture.Sources, func(next port.CredentialStaging) port.Outcome { r.Credentials = next; return "" })
					if !reached || failure != port.CredentialRefused || r.Credentials.Files[port.CredentialIndex(failedKind)] != "" {
						t.Fatal("metadata failure did not preserve refusal and durable authority", failure)
					}
					if kinds.Claude && failedKind == "github-token" && !bytes.Equal(fixture.Read(r.Credentials.Directory+"/claude-token"), values["claude-token"]) {
						t.Fatal("earlier published copy was lost before recovery")
					}
					for range 2 {
						if failure := fixture.Stager.Remove(t.Context(), r); failure != "" {
							t.Fatal("fresh recovery lost partial staging cleanup authority", failure)
						}
					}
					if presence, failure := fixture.Stager.Observe(t.Context(), r); failure != "" || presence != "absent" {
						t.Fatal("metadata failure left staged paths after recovery", failure)
					}
				})
			}
		}
	}
}

func TestCredentialFileMetadataFailurePreservesSubstitution(t *testing.T) {
	for _, replacement := range []string{"file", "symlink", "missing"} {
		t.Run(replacement, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			s := fixture.Stager.(Credentials)
			r := fixture.Intent()
			path := filepath.Join(s.root, filepath.Base(r.Credentials.Directory), "claude-token")
			retained := filepath.Join(t.TempDir(), "retained")
			calls := s.operations()
			stat := calls.stat
			reached := false
			calls.stat = func(file *os.File) (fs.FileInfo, error) {
				if filepath.Base(file.Name()) != "claude-token" {
					return stat(file)
				}
				reached = true
				if err := os.Rename(path, retained); err != nil {
					t.Fatal(err)
				}
				switch replacement {
				case "file":
					if err := os.WriteFile(path, []byte("replacement"), 0400); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					if err := os.Symlink(retained, path); err != nil {
						t.Fatal(err)
					}
				}
				return nil, errors.New("synthetic metadata failure")
			}
			s.ops = &calls
			failure := s.Stage(t.Context(), r, fixture.Sources, func(next port.CredentialStaging) port.Outcome { r.Credentials = next; return "" })
			if !reached || failure != port.CredentialRefused {
				t.Fatal("substitution changed the metadata refusal", failure)
			}
			if _, err := os.Stat(retained); err != nil {
				t.Fatal("cleanup removed the descriptor's renamed file")
			}
			if replacement == "missing" {
				if failure := fixture.Stager.Remove(t.Context(), r); failure != "" {
					t.Fatal("absent unpublished file blocked recovery", failure)
				}
			} else {
				if _, err := os.Lstat(path); err != nil {
					t.Fatal("cleanup removed a substituted entry")
				}
				if failure := fixture.Stager.Remove(t.Context(), r); failure != port.CredentialRefused {
					t.Fatal("recovery accepted a substituted entry", failure)
				}
			}
		})
	}
}
