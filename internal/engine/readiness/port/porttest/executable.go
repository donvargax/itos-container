package porttest

import "github.com/donvargax/itos-container/internal/engine/readiness/port"

// ExecutableControl and the defects name every input of the shared contract.
const ExecutableControl = "a static Linux amd64 control"

// ExecutableDefects are the observations a proxy executable is refused for.
var ExecutableDefects = []string{
	"a resolved executable in the checkout",
	"a symlink resolving into the checkout",
	"ELF with PT_INTERP",
	"ELF with a dynamic dependency",
	"ELF for another machine",
	"ELF with the wrong class",
	"truncated ELF",
	"unavailable executable observation",
}

// Executables is the I/O-free fake: it returns the observation it was given.
type Executables struct {
	Observed port.Executable
	Failure  port.Failure
	Calls    int
}

// Observe counts the call and returns the scripted observation.
func (e *Executables) Observe() (port.Executable, port.Failure) {
	e.Calls++
	return e.Observed, e.Failure
}

// ExpectedExecutable is what the contract requires a backend to observe for the
// named input whose file resolves to path. Another machine and the wrong class
// only have to differ from amd64, so a fake scripts one value for each.
func ExpectedExecutable(name, path string) (port.Executable, port.Failure) {
	observed := port.Executable{Path: path, Regular: true, OS: "linux", Architecture: "amd64"}
	switch name {
	case ExecutableControl, "a resolved executable in the checkout", "a symlink resolving into the checkout":
	case "ELF with PT_INTERP":
		observed.Interpreter, observed.Dynamic = true, true
	case "ELF with a dynamic dependency":
		observed.Dynamic = true
	case "ELF for another machine":
		observed.Architecture = "arm64"
	case "ELF with the wrong class":
		observed.Architecture = "amd64p32"
	default:
		return port.Executable{}, port.EgressExecutable
	}
	return observed, port.None
}

// ExecutableContract runs unchanged against the fake and the ELF adapter.
// create returns the backend for a named input and its file's resolved path.
func ExecutableContract(t Reporter, create func(name string) (port.Executables, string)) {
	t.Helper()
	for _, name := range append([]string{ExecutableControl}, ExecutableDefects...) {
		backend, path := create(name)
		ExecutableCase(t, name, backend, path)
	}
}

// ExecutableCase holds one named input: the same observation twice, complete
// where one exists and empty where none could be made.
func ExecutableCase(t Reporter, name string, backend port.Executables, path string) {
	t.Helper()
	want, failure := ExpectedExecutable(name, path)
	for range 2 {
		got, f := backend.Observe()
		if (f == port.None) != (failure == port.None) {
			t.Errorf("%s: observation failure %v, want %v", name, f, failure)
			continue
		}
		if failure != port.None {
			if got != (port.Executable{}) {
				t.Errorf("%s: a failed observation still carries %+v", name, got)
			}
			continue
		}
		if got.Path != want.Path || got.Regular != want.Regular || got.OS != want.OS || got.Interpreter != want.Interpreter || got.Dynamic != want.Dynamic {
			t.Errorf("%s: observed %+v, want %+v", name, got, want)
		}
		if (got.Architecture == "amd64") != (want.Architecture == "amd64") {
			t.Errorf("%s: observed architecture %q, want %q", name, got.Architecture, want.Architecture)
		}
	}
}
