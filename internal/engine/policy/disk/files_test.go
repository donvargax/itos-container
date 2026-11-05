package disk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func TestPolicySizeBoundary(t *testing.T) {
	for _, size := range []int{65535, 65536, 65537} {
		t.Chdir(t.TempDir())
		want := bytes.Repeat([]byte(" "), size)
		if err := os.WriteFile("itos-container.yaml", want, 0600); err != nil {
			t.Fatal(err)
		}
		data, failure := (Files{}).Read()
		if size > 65536 {
			if data != nil || failure != port.Unreadable {
				t.Fatalf("size %d: got %d bytes, %v; want no data, unreadable", size, len(data), failure)
			}
		} else if failure != port.None || !bytes.Equal(data, want) {
			t.Fatalf("size %d: got %d bytes, %v; want all bytes, no failure", size, len(data), failure)
		}
	}
}

func TestBoundedRead(t *testing.T) {
	// A reader longer than the initial stat size models growth after validation.
	// Check the unread suffix too: rejecting only after reading everything is unsafe.
	reader := strings.NewReader(strings.Repeat("x", 2*65536))
	data, failure := readBounded(reader)
	if data != nil || failure != port.Unreadable || reader.Len() != 65535 {
		t.Fatalf("got %d bytes, %v, %d unread; want no data, unreadable, 65535 unread", len(data), failure, reader.Len())
	}
	for _, size := range []int{0, 65535, 65536} {
		want := strings.Repeat("x", size)
		data, failure := readBounded(strings.NewReader(want))
		if failure != port.None || string(data) != want {
			t.Fatalf("size %d: got %d bytes, %v; want all bytes, no failure", size, len(data), failure)
		}
	}
	readerWithError := io.MultiReader(strings.NewReader("partial data"), iotest.ErrReader(errors.New("read failed")))
	data, failure = readBounded(readerWithError)
	if data != nil || failure != port.Unreadable {
		t.Fatalf("read error returned %q, %v; want no data, unreadable", data, failure)
	}
}

func TestRejectStreamingFiles(t *testing.T) {
	if os.Getenv("SLICE_1_READ_SPECIAL") == "1" {
		data, failure := (Files{}).Read()
		if data != nil || failure != port.Unreadable {
			t.Fatalf("got %d bytes, %v; want no data, unreadable", len(data), failure)
		}
		return
	}

	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := makeFIFO(fifo); errors.Is(err, errors.ErrUnsupported) {
		t.Skip("FIFO and device fixtures require Unix")
	} else if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"fifo", "link-to-fifo", "link-to-device"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "itos-container.yaml")
			var err error
			switch kind {
			case "fifo":
				err = makeFIFO(path)
			case "link-to-fifo":
				err = os.Symlink(fifo, path)
			case "link-to-device":
				err = os.Symlink("/dev/zero", path)
			}
			if err != nil {
				t.Fatal(err)
			}
			// A broken blocking open/read must fail without hanging the test process.
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRejectStreamingFiles$")
			child.Dir = dir
			child.Env = append(os.Environ(), "SLICE_1_READ_SPECIAL=1")
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("special file inspection failed (%v): %v\n%s", ctx.Err(), err, output)
			}
		})
	}
}
