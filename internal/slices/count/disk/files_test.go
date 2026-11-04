package disk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/count/port"
)

func TestFilesReadExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	want := []byte("read only\n")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := (Files{}).Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("Read() = %q, want %q", got, want)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil || string(contents) != string(want) {
		t.Errorf("Read() changed the input to %q, %v", contents, readErr)
	}
}

func TestFilesReadMissingPathAsTypedFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	_, err := (Files{}).Read(path)
	var missing *port.Missing
	if !errors.As(err, &missing) || missing.Path != path {
		t.Fatalf("Read() error = %T %v, want typed Missing for %q", err, err, path)
	}
}

func TestClassifyReadErrorUsesPortFailures(t *testing.T) {
	tests := []struct {
		name        string
		cause       error
		wantMissing bool
	}{
		{name: "missing", cause: fmt.Errorf("open failed: %w", fs.ErrNotExist), wantMissing: true},
		{name: "unreadable", cause: errors.New("read failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := classifyReadError("input", test.cause)
			var missing *port.Missing
			var unreadable *port.Unreadable
			if test.wantMissing && !errors.As(err, &missing) {
				t.Errorf("classification %T = %v, want Missing", err, err)
			}
			if test.wantMissing && missing != nil && missing.Path != "input" {
				t.Errorf("missing path = %q, want input", missing.Path)
			}
			if !test.wantMissing && !errors.As(err, &unreadable) {
				t.Errorf("classification %T = %v, want Unreadable", err, err)
			}
			if !test.wantMissing && unreadable != nil && unreadable.Path != "input" {
				t.Errorf("unreadable path = %q, want input", unreadable.Path)
			}
		})
	}
}
