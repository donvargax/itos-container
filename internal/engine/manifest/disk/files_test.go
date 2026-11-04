package disk

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
)

func TestBoundedRead(t *testing.T) {
	for _, size := range []int{0, 65536, 65537} {
		data, failure := readBounded(strings.NewReader(strings.Repeat("x", size)))
		if size <= 65536 {
			if string(data) != strings.Repeat("x", size) || failure != port.None {
				t.Fatalf("size %d: got %d bytes, %v", size, len(data), failure)
			}
		} else if data != nil || failure != port.Complexity {
			t.Fatalf("oversized read: got %d bytes, %v", len(data), failure)
		}
	}
	// A source that never reaches EOF models growth after the descriptor's Stat.
	reader := &growingReader{}
	if data, failure := readBounded(reader); data != nil || failure != port.Complexity || reader.bytes != 65537 {
		t.Fatalf("growing read: %d bytes returned, %v, %d bytes consumed", len(data), failure, reader.bytes)
	}
	if data, failure := readBounded(io.MultiReader(strings.NewReader("partial"), failedReader{})); data != nil || failure != port.Path {
		t.Fatalf("read error returned partial data: %q, %v", data, failure)
	}
}

type growingReader struct{ bytes int }

func (r *growingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.bytes += len(p)
	return len(p), nil
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }

func TestFilesContract(t *testing.T) {
	porttest.FilesContract(t, func(data []byte, failure port.Failure) port.Files {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "worker.json")
		var err error
		switch failure {
		case port.None:
			err = os.WriteFile(path, data, 0o600)
		case port.Path:
			err = os.Mkdir(path, 0o700)
		}
		if err != nil {
			t.Fatal(err)
		}
		return Files{Root: root}
	})
}

func TestNoFollowAndPathFailures(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real", "worker.json"), []byte("bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"link": "real", "dangling": "absent", "file": "real/worker.json"} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	f := Files{Root: root}
	for _, path := range []string{"../escape", "/absolute", "a\\b", "a\x00b", ".", "link/worker.json", "dangling/worker.json", "file", "real/worker.json/child"} {
		if data, failure := f.Read(path); data != nil || failure != port.Path {
			t.Errorf("Read(%q) = %q, %v", path, data, failure)
		}
		if f.Directory(path) {
			t.Errorf("Directory(%q) accepted", path)
		}
	}
	if data, failure := f.Read("missing/worker.json"); data != nil || failure != port.Missing {
		t.Fatal(data, failure)
	}
	if f.Directory("missing") || !f.Directory("real/nested") {
		t.Fatal("directory existence")
	}
	if data, failure := f.Read("real/worker.json"); string(data) != "bytes" || failure != port.None {
		t.Fatal(data, failure)
	}
	t.Chdir(root)
	if data, failure := (Files{}).Read("real/worker.json"); string(data) != "bytes" || failure != port.None {
		t.Fatal(data, failure)
	}
	missing := Files{Root: filepath.Join(root, "absent")}
	if data, failure := missing.Read("worker.json"); data != nil || failure != port.Path {
		t.Fatal(data, failure)
	}
	if missing.Directory("src") {
		t.Fatal("absent root accepted")
	}
	if fileFailure(fs.ErrNotExist) != port.Missing || fileFailure(errors.New("unreadable")) != port.Path {
		t.Fatal("error mapping")
	}
}
