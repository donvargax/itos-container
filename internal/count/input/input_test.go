package input_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/count/input"
	"github.com/donvargax/itos-container/internal/count/port"
	"github.com/donvargax/itos-container/internal/count/port/porttest"
)

// Standard input is returned whole, with its bytes unchanged.
func TestFilesReadStandardInputForTheDash(t *testing.T) {
	files := input.Files{Disk: porttest.Files{}, In: strings.NewReader("one two\nthree")}
	got, err := files.Read("-")
	if err != nil || string(got) != "one two\nthree" {
		t.Fatalf("Read(-) = %q, %v; want original standard-input bytes", got, err)
	}
}

// A named path is the disk's, never the reader's.
func TestFilesReadANamedPathThroughTheDisk(t *testing.T) {
	files := input.Files{
		Disk: porttest.Files{Data: map[string][]byte{"notes": []byte("one two")}},
		In:   strings.NewReader("ignored"),
	}
	got, err := files.Read("notes")
	if err != nil || string(got) != "one two" {
		t.Fatalf("Read(notes) = %q, %v; want disk bytes", got, err)
	}
}

// A path the disk has no file for is its failure, the reader's never spoken
// for: the argument named a file, so it is a missing file.
func TestFilesNameAMissingFileThroughTheDisk(t *testing.T) {
	files := input.Files{Disk: porttest.Files{}, In: strings.NewReader("data")}
	_, err := files.Read("absent")
	var missing *port.Missing
	if !errors.As(err, &missing) {
		t.Fatalf("Read(absent) = %T %v, want typed Missing", err, err)
	}
}

// A reader that fails is the environment's: unreadable, as a file that
// cannot be read is. No permission bit is assumed, so this holds wherever
// the reader fails.
func TestFilesReportAFailingReaderAsUnreadable(t *testing.T) {
	files := input.Files{Disk: porttest.Files{}, In: failingReader{}}
	_, err := files.Read("-")
	var unreadable *port.Unreadable
	if !errors.As(err, &unreadable) || unreadable.Path != "-" {
		t.Fatalf("Read(-, failing reader) = %T %v, want typed Unreadable for -", err, err)
	}
}

// failingReader is a reader that always fails, an environment the tests can
// make wherever a file cannot be made unreadable.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("cannot read") }
