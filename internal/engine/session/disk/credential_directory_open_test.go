package disk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestCredentialDirectoryOpenFailurePreservesSubstitution(t *testing.T) {
	for _, replacement := range []string{"directory", "symlink", "file", "missing", "extra entry"} {
		t.Run(replacement, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			s := fixture.Stager.(Credentials)
			r := fixture.Intent()
			path := filepath.Join(s.root, filepath.Base(r.Credentials.Directory))
			retained := filepath.Join(s.root, "retained")
			calls := s.operations()
			reached := false
			var original, substituted fs.FileInfo
			calls.openFile = func(root *os.Root, name string, _ int, _ fs.FileMode) (*os.File, error) {
				reached = true
				var err error
				original, err = root.Lstat(name)
				if err != nil || !original.IsDir() {
					t.Fatal("fixture did not reach the newly created directory")
				}
				if replacement == "extra entry" {
					err = os.WriteFile(filepath.Join(path, "unexpected"), []byte("retain"), 0400)
				} else {
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
				if replacement != "missing" {
					substituted, err = os.Lstat(path)
					if err != nil {
						t.Fatal(err)
					}
				}
				return nil, errors.New("synthetic directory open failure")
			}
			s.ops = &calls
			if failure := s.Stage(t.Context(), r, fixture.Sources, func(port.CredentialStaging) port.Outcome {
				t.Fatal("failed directory open published staging authority")
				return ""
			}); failure != port.CredentialRefused || !reached {
				t.Fatal("directory open did not retain its refusal", failure)
			}
			for range 2 {
				if replacement == "missing" {
					if failure := fixture.Stager.Remove(t.Context(), r); failure != "" {
						t.Fatal("absent staging blocked repeated recovery", failure)
					}
					continue
				}
				if failure := fixture.Stager.Remove(t.Context(), r); failure != port.CredentialRefused {
					t.Fatal("intent-only recovery adopted substituted staging", failure)
				}
				if current, err := os.Lstat(path); err != nil || !os.SameFile(current, substituted) {
					t.Fatal("cleanup removed or changed the substituted entry")
				}
			}
			if replacement == "extra entry" {
				if data, err := os.ReadFile(filepath.Join(path, "unexpected")); err != nil || string(data) != "retain" {
					t.Fatal("cleanup changed an unexpected entry")
				}
			} else if current, err := os.Lstat(retained); err != nil || !os.SameFile(current, original) {
				t.Fatal("cleanup changed the renamed original directory")
			}
		})
	}
}
