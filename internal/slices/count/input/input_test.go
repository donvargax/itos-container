package input_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/count/domain"
	"github.com/donvargax/itos-container/internal/slices/count/input"
	"github.com/donvargax/itos-container/internal/slices/count/port"
	"github.com/donvargax/itos-container/internal/slices/count/port/porttest"
)

// Standard input is read whole, as a file is: the counts are the domain's,
// whichever the path came from.
func TestFilesReadStandardInputForTheDash(t *testing.T) {
	files := input.Files{Disk: porttest.Files{}, In: strings.NewReader("one two\nthree")}
	got, err := domain.File(files, "-")
	if err != nil || got != (domain.Result{Lines: 2, Words: 3}) {
		t.Fatalf("File(-, …) = %+v, %v; want 2 lines and 3 words", got, err)
	}
}

// A named path is the disk's, never the reader's.
func TestFilesReadANamedPathThroughTheDisk(t *testing.T) {
	files := input.Files{
		Disk: porttest.Files{Data: map[string][]byte{"notes": []byte("one two")}},
		In:   strings.NewReader("ignored"),
	}
	got, err := domain.File(files, "notes")
	if err != nil || got != (domain.Result{Lines: 1, Words: 2}) {
		t.Fatalf("File(notes, …) = %+v, %v; want 1 line and 2 words", got, err)
	}
}

// A path the disk has no file for is its failure, the reader's never spoken
// for: the argument named a file, so it is a missing file.
func TestFilesNameAMissingFileThroughTheDisk(t *testing.T) {
	files := input.Files{Disk: porttest.Files{}, In: strings.NewReader("data")}
	_, err := domain.File(files, "absent")
	var missing *port.Missing
	if !errors.As(err, &missing) {
		t.Fatalf("File(absent, …) = %T %v, want typed Missing", err, err)
	}
}

// A reader that fails is the environment's: unreadable, as a file that
// cannot be read is. No permission bit is assumed, so this holds wherever
// the reader fails.
func TestFilesReportAFailingReaderAsUnreadable(t *testing.T) {
	files := input.Files{Disk: porttest.Files{}, In: failingReader{}}
	_, err := domain.File(files, "-")
	var unreadable *port.Unreadable
	if !errors.As(err, &unreadable) || unreadable.Path != "-" {
		t.Fatalf("File(-, failing reader) = %T %v, want typed Unreadable for -", err, err)
	}
}

// failingReader is a reader that always fails, an environment the tests can
// make wherever a file cannot be made unreadable.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("cannot read") }
