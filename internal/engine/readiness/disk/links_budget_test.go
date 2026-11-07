package disk

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func scanRoot(t *testing.T, path string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestCheckoutScanCountsAllEntriesAcrossDirectoriesAndBatches(t *testing.T) {
	path := t.TempDir()
	// 3 directory entries, 3 dangling symlinks and 390 regular files. Each
	// directory needs multiple reads, and none may reset the total allowance.
	for i := 0; i < 3; i++ {
		dir := filepath.Join(path, strconv.Itoa(i))
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 130; j++ {
			mustWrite(t, filepath.Join(dir, strconv.Itoa(j)))
		}
		if err := os.Symlink("missing", filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
	}
	root := scanRoot(t, path)
	for _, tc := range []struct {
		entries int
		want    port.Failure
	}{{395, port.CheckoutUnavailable}, {396, port.None}, {397, port.None}} {
		s := newFileScan()
		s.remaining = tc.entries
		reads := 0
		s.readDir = func(f *os.File, n int) ([]os.DirEntry, error) {
			reads++
			if n <= 0 || n > 128 {
				t.Fatalf("unbounded directory read: %d", n)
			}
			return f.ReadDir(n)
		}
		if got := s.inspectFiles(root, 0, (*os.Root).Open); got != tc.want {
			t.Fatalf("%d entries: got %v, want %v", tc.entries, got, tc.want)
		}
		if reads < 4 || (tc.want == port.None && s.remaining != tc.entries-396) {
			t.Fatalf("incomplete scan: reads=%d remaining=%d", reads, s.remaining)
		}
	}
}

func TestCheckoutScanDepthBoundaryAndFreshRechecks(t *testing.T) {
	path := checkoutFixture(t)
	dir := path
	for i := 0; i < 64; i++ {
		dir = filepath.Join(dir, "d")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	c := &Checkout{}
	t.Cleanup(c.Close)
	if got := c.Pin(path); got != port.None {
		t.Fatal(got)
	}
	if _, got := c.Inspect(); got != port.None || !c.Unchanged() || !c.Unchanged() {
		t.Fatal("depth 64 refused or recheck reused exhausted budget", got)
	}
	if err := os.Mkdir(filepath.Join(dir, "over"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, got := c.Inspect(); got != port.CheckoutUnavailable || c.Unchanged() {
		t.Fatal("depth 65 admitted", got)
	}
}

func TestCheckoutScanDeadline(t *testing.T) {
	root := scanRoot(t, t.TempDir())
	for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		s := newFileScan()
		s.now = func() time.Time { return s.deadline.Add(offset) }
		want := port.CheckoutUnavailable
		if offset < 0 {
			want = port.None
		}
		if got := s.inspectFiles(root, 0, (*os.Root).Open); got != want {
			t.Fatalf("deadline offset %s: got %v, want %v", offset, got, want)
		}
	}
}

func TestCheckoutScanTimeSpentInFilesystemCalls(t *testing.T) {
	for _, stage := range []string{"open", "read empty", "read entry", "child open"} {
		t.Run(stage, func(t *testing.T) {
			path := t.TempDir()
			if stage == "read entry" {
				mustWrite(t, filepath.Join(path, "file"))
			}
			root := scanRoot(t, path)
			s := newFileScan()
			now := s.deadline.Add(-time.Nanosecond)
			s.now = func() time.Time { return now }
			open := (*os.Root).Open
			if stage == "open" {
				open = func(r *os.Root, name string) (*os.File, error) {
					file, err := r.Open(name)
					now = s.deadline
					return file, err
				}
			}
			s.readDir = func(f *os.File, n int) ([]os.DirEntry, error) {
				entries, err := f.ReadDir(n)
				now = s.deadline
				return entries, err
			}
			var got port.Failure
			if stage == "child open" {
				before, err := root.Stat(".")
				if err != nil {
					t.Fatal(err)
				}
				got = s.inspectDirectory(root, ".", before, 1, func(r *os.Root, name string) (*os.Root, error) {
					child, err := r.OpenRoot(name)
					now = s.deadline
					return child, err
				})
			} else {
				got = s.inspectFiles(root, 0, open)
			}
			if got != port.CheckoutUnavailable {
				t.Fatal("expired scan admitted", got)
			}
		})
	}
}

func TestCheckoutScanSharesDeadlineWithChildren(t *testing.T) {
	path := t.TempDir()
	if err := os.Mkdir(filepath.Join(path, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	root := scanRoot(t, path)
	s := newFileScan()
	now := s.deadline.Add(-time.Nanosecond)
	s.now = func() time.Time { return now }
	s.readDir = func(f *os.File, n int) ([]os.DirEntry, error) {
		entries, err := f.ReadDir(n)
		// The child's empty directory read exhausts the parent's deadline too.
		if err == io.EOF {
			now = s.deadline
		}
		return entries, err
	}
	if got := s.inspectFiles(root, 0, (*os.Root).Open); got != port.CheckoutUnavailable {
		t.Fatal("child reset the deadline", got)
	}
}
