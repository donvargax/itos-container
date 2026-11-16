package porttest

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestExecutableContractAgainstTheFake(t *testing.T) {
	ExecutableContract(t, func(name string) (port.Executables, string) {
		path := "/installed/itos-container"
		observed, failure := ExpectedExecutable(name, path)
		return &Executables{Observed: observed, Failure: failure}, path
	})
}

type executableFailures struct{ count int }

func (*executableFailures) Helper()                  {}
func (r *executableFailures) Errorf(string, ...any)  { r.count++ }
func (r *executableFailures) Fatalf(string, ...any)  { r.count++ }
func (r *executableFailures) failed(run func()) bool { r.count = 0; run(); return r.count != 0 }

// The contract must tell a backend that misreads an input from one that reads it.
func TestExecutableCaseRejectsAMisreadInput(t *testing.T) {
	path := "/installed/itos-container"
	static, _ := ExpectedExecutable(ExecutableControl, path)
	reporter := &executableFailures{}
	for _, name := range ExecutableDefects[2:] {
		if !reporter.failed(func() { ExecutableCase(reporter, name, &Executables{Observed: static}, path) }) {
			t.Errorf("%s: a static amd64 observation satisfied the contract", name)
		}
	}
	for _, wrong := range []Executables{
		{Failure: port.EgressExecutable},
		{Observed: port.Executable{Path: "/elsewhere", Regular: true, OS: "linux", Architecture: "amd64"}},
		{Observed: port.Executable{Path: path, OS: "linux", Architecture: "amd64"}},
		{Observed: port.Executable{Path: path, Regular: true, OS: "freebsd", Architecture: "amd64"}},
	} {
		backend := wrong
		if !reporter.failed(func() { ExecutableCase(reporter, ExecutableControl, &backend, path) }) {
			t.Errorf("control: %+v satisfied the contract", wrong)
		}
	}
	unavailable := &Executables{Observed: static, Failure: port.EgressExecutable}
	if !reporter.failed(func() { ExecutableCase(reporter, "truncated ELF", unavailable, path) }) {
		t.Error("a failed observation that still carried a reading satisfied the contract")
	}
}
