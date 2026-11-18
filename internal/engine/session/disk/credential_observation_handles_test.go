package disk

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestCredentialObservationReleasesHandles(t *testing.T) {
	for _, presence := range []string{"present", "absent", "unknown"} {
		t.Run(presence, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			r, failure := stageCredentialFixture(t, fixture)
			if failure != "" {
				t.Fatal(failure)
			}
			want := port.Outcome("")
			switch presence {
			case "absent":
				if failure := fixture.Stager.Remove(t.Context(), r); failure != "" {
					t.Fatal(failure)
				}
			case "unknown":
				r.Credentials.Inode += "0"
				want = port.CredentialRefused
			}
			s := fixture.Stager.(Credentials)
			calls := s.operations()
			openRoot, openFile := calls.openRoot, calls.openFile
			var roots []*os.Root
			var files []*os.File
			calls.openRoot = func(path string) (*os.Root, error) {
				root, err := openRoot(path)
				if err == nil {
					roots = append(roots, root)
					t.Cleanup(func() { _ = root.Close() })
				}
				return root, err
			}
			calls.openFile = func(root *os.Root, name string, flags int, mode fs.FileMode) (*os.File, error) {
				file, err := openFile(root, name, flags, mode)
				if err == nil {
					files = append(files, file)
					t.Cleanup(func() { _ = file.Close() })
				}
				return file, err
			}
			s.ops = &calls
			if got, failure := s.Observe(t.Context(), r); got != presence || failure != want {
				t.Fatal("observation changed its presence or refusal", got, failure)
			}
			if len(roots) == 0 || presence != "absent" && len(files) == 0 {
				t.Fatal("fixture did not acquire observation handles")
			}
			for _, root := range roots {
				if _, err := root.Stat("."); !errors.Is(err, os.ErrClosed) {
					t.Error("observation retained an open directory root")
				}
			}
			for _, file := range files {
				if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
					t.Error("observation retained an open directory descriptor")
				}
			}
		})
	}
}
