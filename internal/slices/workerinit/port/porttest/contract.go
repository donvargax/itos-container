package porttest

import (
	"errors"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
)

// Reporter is the testing output the contracts use. A recording fake lets
// their tests verify that faulty ports trigger each assertion.
type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// MinABI is the first Landlock ABI that handles pathname sockets.
const MinABI = 9

// LandlockWant is what a Landlock under test must do: Unavailable when the
// kernel has no Landlock, otherwise the ABI an independent measurement found.
type LandlockWant struct {
	Unavailable bool
	ABI         int
}

// LandlockContract runs the same assertions against the fake and the real
// adapter. Restrict is called once, so the caller's thread must be one the
// test may restrict.
func LandlockContract(t Reporter, open func() (port.Landlock, LandlockWant)) {
	t.Helper()
	l, want := open()
	abi, err := l.ABI()
	again, errAgain := l.ABI()
	restrictErr := l.Restrict()
	if want.Unavailable {
		if !errors.Is(err, port.ErrUnavailable) || !errors.Is(errAgain, port.ErrUnavailable) {
			t.Errorf("ABI() errors = %v, %v; want ErrUnavailable", err, errAgain)
		}
		if !errors.Is(restrictErr, port.ErrUnavailable) {
			t.Errorf("Restrict() = %v; want ErrUnavailable", restrictErr)
		}
		return
	}
	if err != nil || errAgain != nil || abi != want.ABI || again != abi {
		t.Errorf("ABI() = %d, %v then %d, %v; want %d, nil twice", abi, err, again, errAgain, want.ABI)
	}
	if want.ABI >= MinABI && restrictErr != nil {
		t.Errorf("Restrict() = %v on ABI %d; want nil", restrictErr, want.ABI)
	}
	if want.ABI < MinABI && restrictErr == nil {
		t.Errorf("Restrict() = nil on ABI %d; want an error", want.ABI)
	}
}

// ProcessWant names a bare program PATH finds with the path it resolves to,
// a bare one it does not find, and a path with a slash that does not exist.
type ProcessWant struct {
	Bare, Resolved, Missing, MissingPath string
}

// ProcessContract runs the same lookup assertions against the fake and the
// real adapter. It never replaces the process: Exec is called only for a path
// that is not there.
func ProcessContract(t Reporter, open func() (port.Process, ProcessWant)) {
	t.Helper()
	p, want := open()
	if got, err := p.Lookup(want.Bare); err != nil || got != want.Resolved {
		t.Errorf("Lookup(%q) = %q, %v; want %q, nil", want.Bare, got, err, want.Resolved)
	}
	if got, err := p.Lookup(want.Missing); !errors.Is(err, port.ErrNotFound) || got != "" {
		t.Errorf("Lookup(%q) = %q, %v; want empty, ErrNotFound", want.Missing, got, err)
	}
	if got, err := p.Lookup(want.MissingPath); err != nil || got != want.MissingPath {
		t.Errorf("Lookup(%q) = %q, %v; want it unchanged", want.MissingPath, got, err)
	}
	if err := p.Exec(want.MissingPath, []string{want.MissingPath}); !errors.Is(err, port.ErrNotFound) {
		t.Errorf("Exec(%q) = %v; want ErrNotFound", want.MissingPath, err)
	}
}
