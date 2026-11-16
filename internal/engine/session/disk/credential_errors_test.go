package disk

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func credentialFaultOperations(s Credentials, operation string, at int) credentialOperations {
	calls := s.operations()
	count := 0
	fault := func(name string) bool {
		if name != operation {
			return false
		}
		count++
		return count == at
	}
	err := errors.New("synthetic credential filesystem failure")
	openRoot, openFile, stat, names := calls.openRoot, calls.openFile, calls.stat, calls.names
	calls.openRoot = func(path string) (*os.Root, error) {
		if fault("root") {
			return nil, err
		}
		return openRoot(path)
	}
	calls.openFile = func(root *os.Root, name string, flags int, mode fs.FileMode) (*os.File, error) {
		if fault("file") {
			return nil, err
		}
		return openFile(root, name, flags, mode)
	}
	calls.stat = func(file *os.File) (fs.FileInfo, error) {
		if fault("stat") {
			return nil, err
		}
		return stat(file)
	}
	calls.names = func(file *os.File, limit int) ([]string, error) {
		if fault("names") {
			return nil, err
		}
		return names(file, limit)
	}
	return calls
}

func TestCredentialStagingFilesystemErrors(t *testing.T) {
	for _, test := range []struct {
		name, operation string
		at              int
		want            port.Outcome
	}{
		{"base open", "root", 1, port.CredentialUnavailable},
		{"directory open", "file", 1, port.CredentialRefused},
		{"directory stat", "stat", 1, port.CredentialRefused},
		{"descriptor root", "root", 2, port.CredentialUnavailable},
		{"source stat", "stat", 2, port.CredentialRefused},
		{"source restat", "stat", 3, port.CredentialRefused},
		{"file create", "file", 2, port.CredentialRefused},
		{"file stat", "stat", 4, port.CredentialRefused},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			s := fixture.Stager.(Credentials)
			calls := credentialFaultOperations(s, test.operation, test.at)
			s.ops = &calls
			r := fixture.Intent()
			failure := s.Stage(t.Context(), r, fixture.Sources, func(next port.CredentialStaging) port.Outcome { r.Credentials = next; return "" })
			if failure != test.want {
				t.Fatal("failed filesystem operation did not refuse staging", failure)
			}
			if r.Credentials.Stage == "owned" && test.name != "file stat" {
				if failure := fixture.Stager.Remove(t.Context(), r); failure != "" {
					t.Fatal("published partial staging lost cleanup authority", failure)
				}
			}
		})
	}
}

func TestCredentialObservationFilesystemErrors(t *testing.T) {
	for _, test := range []struct {
		name, operation string
		at              int
		want            port.Outcome
	}{
		{"base open", "root", 1, port.CredentialUnavailable},
		{"directory stat", "stat", 1, port.CredentialRefused},
		{"descriptor root", "root", 2, port.CredentialRefused},
		{"directory read", "names", 1, port.CredentialRefused},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			r, failure := stageCredentialFixture(t, fixture)
			if failure != "" {
				t.Fatal("observation positive control failed", failure)
			}
			s := fixture.Stager.(Credentials)
			calls := credentialFaultOperations(s, test.operation, test.at)
			s.ops = &calls
			if presence, failure := s.Observe(t.Context(), r); presence != "unknown" || failure != test.want {
				t.Fatal("failed observation invented staging presence or absence", presence, failure)
			}
			if !fixture.Present(r.Credentials.Directory) {
				t.Fatal("read-only observation removed staging")
			}
			if failure := fixture.Stager.Remove(t.Context(), r); failure != "" {
				t.Fatal("fresh observation could not recover staging", failure)
			}
		})
	}
}

type credentialSourceSelection struct {
	port.CredentialSources
	entries []port.CheckedCredential
	before  func()
}

func (s credentialSourceSelection) Checked() []port.CheckedCredential {
	if s.before != nil {
		s.before()
	}
	return s.entries
}

func TestCredentialStagingRejectsInvalidCapabilities(t *testing.T) {
	for _, fault := range []string{"invalid intent", "nil sources", "wrong kinds", "missing entry", "unknown kind", "wrong path", "duplicate kind", "invalid descriptor", "missing descriptor"} {
		t.Run(fault, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n"), "github-token": []byte("another\n")})
			r := fixture.Intent()
			selected := credentialSourceSelection{CredentialSources: fixture.Sources, entries: fixture.Sources.Checked()}
			var sources port.CredentialSources = selected
			switch fault {
			case "invalid intent":
				r.Credentials.Stage = "owned"
			case "nil sources":
				sources = nil
			case "wrong kinds":
				r.Credentials.Kinds.GitHub = false
			case "missing entry":
				selected.entries = selected.entries[:1]
			case "unknown kind":
				selected.entries[0].Kind = "unknown"
			case "wrong path":
				selected.entries[0].ReadPath = "/original/token"
			case "duplicate kind":
				selected.entries[1].Kind = selected.entries[0].Kind
			case "invalid descriptor":
				selected.entries[0].ReadPath = "/proc/self/fd/00"
			case "missing descriptor":
				selected.entries[0].ReadPath = "/proc/self/fd/999999999"
			}
			if sources != nil {
				sources = selected
			}
			if failure := fixture.Stager.Stage(t.Context(), r, sources, func(port.CredentialStaging) port.Outcome { return "" }); failure != port.CredentialRefused {
				t.Fatal("invalid credential capability reached a successful copy", failure)
			}
		})
	}
}

func TestCredentialStagingCancellationBoundaries(t *testing.T) {
	for _, boundary := range []string{"checked", "mkdir", "root", "read", "file", "copy", "write"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			s := fixture.Stager.(Credentials)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			selected := credentialSourceSelection{CredentialSources: fixture.Sources, entries: fixture.Sources.Checked()}
			if boundary == "checked" {
				selected.before = cancel
			}
			s.before = func(event string) {
				if event == boundary {
					cancel()
				}
			}
			calls := s.operations()
			openRoot := calls.openRoot
			calls.openRoot = func(path string) (*os.Root, error) {
				if boundary == "root" && path != s.root {
					cancel()
				}
				return openRoot(path)
			}
			s.ops = &calls
			s.write = func(file *os.File, data []byte) (int, error) {
				n, err := file.Write(data)
				if boundary == "write" {
					cancel()
				}
				return n, err
			}
			r := fixture.Intent()
			if failure := s.Stage(ctx, r, selected, func(next port.CredentialStaging) port.Outcome { r.Credentials = next; return "" }); failure == "" {
				t.Fatal("cancelled staging reported a successful copy")
			}
			if fixture.Present(r.Credentials.Directory) {
				t.Fatal("cancelled staging left a copy after its syscall returned")
			}
		})
	}
}

func TestCredentialCleanupLateFilesystemChanges(t *testing.T) {
	for _, change := range []string{"cancelled", "missing directory", "new entry"} {
		t.Run(change, func(t *testing.T) {
			fixture := diskCredentialFixture(t, map[string][]byte{"claude-token": []byte("synthetic\n")})
			r, failure := stageCredentialFixture(t, fixture)
			if failure != "" {
				t.Fatal(failure)
			}
			s := fixture.Stager.(Credentials)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			s.before = func(string) {
				if change == "cancelled" {
					cancel()
				}
			}
			path := filepath.Join(s.root, filepath.Base(r.Credentials.Directory))
			s.unlink = func(root *os.Root, name string) error {
				if err := root.Remove(name); err != nil {
					return err
				}
				if change == "missing directory" {
					return os.Remove(path)
				}
				return os.WriteFile(filepath.Join(path, "unexpected"), nil, 0400)
			}
			if failure := s.Remove(ctx, r); failure == "" {
				t.Fatal("cleanup ignored a late cancellation or filesystem change")
			}
		})
	}
}

func TestCredentialMetadataUnavailable(t *testing.T) {
	if _, ok := credentialNode(nil); ok {
		t.Fatal("missing metadata supplied an identity")
	}
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []any{nil, "unsupported", struct{ Dev uint64 }{1}, struct{ Dev int64 }{1}} {
		if _, ok := credentialNode(credentialStatInfo{info, raw}); ok {
			t.Fatal("unavailable native metadata supplied an identity")
		}
	}
}
