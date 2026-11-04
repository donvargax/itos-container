package porttest

import (
	"fmt"
	"github.com/donvargax/itos-container/internal/slices/count/port"
	"strings"
	"testing"
)

func TestFilesContract(t *testing.T) {
	FilesContract(t, func() (port.Files, Fixture) { return ContractFiles() })
}

type recordingReporter struct{ errors []string }

func (*recordingReporter) Helper() {}
func (r *recordingReporter) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

type faultyFiles struct {
	Files
	fault  string
	reads  map[string]int
	shared []byte
}

func (f *faultyFiles) Read(path string) ([]byte, port.ReadFailure) {
	f.reads[path]++
	n := f.reads[path]
	data, failure := f.Files.Read(path)
	if path == "data" {
		switch f.fault {
		case "first-bytes":
			if n == 1 {
				return []byte("wrong"), nil
			}
		case "first-failure":
			if n == 1 {
				return data, &port.Unreadable{Path: path}
			}
		case "repeat-bytes":
			if n == 2 {
				return []byte("wrong"), nil
			}
		case "repeat-failure":
			if n == 2 {
				return data, &port.Unreadable{Path: path}
			}
		case "shared-storage":
			if f.shared == nil {
				f.shared = data
			}
			return f.shared, nil
		case "partial-overlap":
			if f.shared == nil {
				f.shared = make([]byte, 15)
			}
			if n == 1 {
				copy(f.shared, data)
				return f.shared[:8], nil
			}
			if n == 2 {
				copy(f.shared[7:], data)
				return f.shared[7:], nil
			}
		case "retained-mutation":
			if n == 1 {
				f.shared = data
				return data, nil
			}
			if n == 3 {
				return f.shared, nil
			}
		case "third-failure":
			if n == 3 {
				return data, &port.Unreadable{Path: path}
			}
		}
	}
	if path == "missing" {
		switch f.fault {
		case "failure-data":
			return []byte("unexpected"), failure
		case "missing-type":
			return nil, &port.Unreadable{Path: path}
		case "missing-path":
			return nil, &port.Missing{Path: "other"}
		}
	}
	if path == "directory" {
		switch f.fault {
		case "unreadable-type":
			return nil, &port.Missing{Path: path}
		case "unreadable-path":
			return nil, &port.Unreadable{Path: "other"}
		}
	}
	return data, failure
}

func TestFilesContractDetectsBrokenImplementations(t *testing.T) {
	for fault, want := range map[string]string{
		"first-bytes": "Read(\"data\")", "first-failure": "Read(\"data\")",
		"repeat-bytes": "repeat Read", "repeat-failure": "repeat Read",
		"shared-storage": "shares returned storage", "partial-overlap": "shares returned storage",
		"retained-mutation": "retained caller mutation", "third-failure": "retained caller mutation",
		"failure-data": "returned failure data", "missing-type": "want Missing", "missing-path": "want Missing",
		"unreadable-type": "want Unreadable", "unreadable-path": "want Unreadable",
	} {
		t.Run(fault, func(t *testing.T) {
			reporter := &recordingReporter{}
			FilesContract(reporter, func() (port.Files, Fixture) {
				files, paths := ContractFiles()
				return &faultyFiles{Files: files, fault: fault, reads: map[string]int{}}, paths
			})
			if !strings.Contains(strings.Join(reporter.errors, "\n"), want) {
				t.Fatalf("contract missed %s: %v", fault, reporter.errors)
			}
		})
	}
}
