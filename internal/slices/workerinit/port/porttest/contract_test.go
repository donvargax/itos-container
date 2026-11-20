package porttest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
)

type recordingReporter struct{ errors []string }

func (*recordingReporter) Helper() {}
func (r *recordingReporter) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestLandlockContractAcceptsTheFake(t *testing.T) {
	for _, l := range []*Landlock{
		{Version: 10}, {Version: 9}, {Version: 8}, {VersionErr: port.ErrUnavailable},
	} {
		l.Rec = &Recorder{}
		want := LandlockWant{ABI: l.Version, Unavailable: l.VersionErr != nil}
		r := &recordingReporter{}
		LandlockContract(r, func() (port.Landlock, LandlockWant) { return l, want })
		if len(r.errors) != 0 {
			t.Errorf("fake %+v refused: %v", l, r.errors)
		}
	}
}

// liar is a Landlock that answers as configured, whatever the kernel says.
type liar struct {
	abi         int
	abiErr      error
	restrictErr error
}

func (l liar) ABI() (int, error) { return l.abi, l.abiErr }
func (l liar) Restrict() error   { return l.restrictErr }

func TestLandlockContractRejectsFaultyPorts(t *testing.T) {
	boom := errors.New("boom")
	for _, c := range []struct {
		name string
		l    liar
		want LandlockWant
		errs int
	}{
		{"unavailable reported as an ABI", liar{abi: 10}, LandlockWant{Unavailable: true}, 2},
		{"unavailable restrict succeeds", liar{abiErr: port.ErrUnavailable}, LandlockWant{Unavailable: true}, 1},
		{"wrong ABI", liar{abi: 9}, LandlockWant{ABI: 10}, 1},
		{"ABI error", liar{abiErr: boom}, LandlockWant{ABI: 10}, 1},
		{"restrict fails on a capable kernel", liar{abi: 10, restrictErr: boom}, LandlockWant{ABI: 10}, 1},
		{"restrict succeeds below ABI 9", liar{abi: 8}, LandlockWant{ABI: 8}, 1},
	} {
		r := &recordingReporter{}
		LandlockContract(r, func() (port.Landlock, LandlockWant) { return c.l, c.want })
		if len(r.errors) != c.errs {
			t.Errorf("%s: %d assertions failed %v; want %d", c.name, len(r.errors), r.errors, c.errs)
		}
	}
}

// brokenProcess answers as configured, whatever PATH says.
type brokenProcess struct {
	resolved, found string
	missing         error
	exec            error
}

func (p brokenProcess) Lookup(name string) (string, error) {
	switch name {
	case "bare":
		return p.found, nil
	case "absent":
		return "", p.missing
	}
	return p.resolved, nil
}
func (p brokenProcess) Exec(string, []string) error { return p.exec }

func TestProcessContractRejectsFaultyPorts(t *testing.T) {
	want := ProcessWant{Bare: "bare", Resolved: "/bin/bare", Missing: "absent", MissingPath: "/no/path"}
	good := brokenProcess{found: "/bin/bare", resolved: "/no/path", missing: port.ErrNotFound, exec: port.ErrNotFound}
	for _, c := range []struct {
		name string
		p    brokenProcess
		errs int
	}{
		{"correct", good, 0},
		{"wrong bare path", brokenProcess{found: "/other", resolved: good.resolved, missing: good.missing, exec: good.exec}, 1},
		{"missing name found", brokenProcess{found: good.found, resolved: good.resolved, exec: good.exec}, 1},
		{"slash path changed", brokenProcess{found: good.found, resolved: "/elsewhere", missing: good.missing, exec: good.exec}, 1},
		{"exec of a missing path succeeds", brokenProcess{found: good.found, resolved: good.resolved, missing: good.missing}, 1},
	} {
		r := &recordingReporter{}
		ProcessContract(r, func() (port.Process, ProcessWant) { return c.p, want })
		if len(r.errors) != c.errs {
			t.Errorf("%s: %d assertions failed %v; want %d", c.name, len(r.errors), r.errors, c.errs)
		}
	}
}

func TestFakesRecordTheirCalls(t *testing.T) {
	rec := &Recorder{}
	p := &Process{Rec: rec, Bare: map[string]string{"x": "/bin/x"}, Files: map[string]bool{"/bin/x": true}}
	if path, err := p.Lookup("x"); path != "/bin/x" || err != nil {
		t.Errorf("Lookup(x) = %q, %v", path, err)
	}
	if err := p.Exec("/bin/x", []string{"x"}); err != nil {
		t.Errorf("Exec of a present file = %v; want nil", err)
	}
	if err := p.Exec("/bin/y", nil); !errors.Is(err, port.ErrNotFound) {
		t.Errorf("Exec of a missing file = %v; want ErrNotFound", err)
	}
	l := &Landlock{Rec: rec, Version: 10, RestrictErr: errors.New("no")}
	if err := l.Restrict(); err == nil {
		t.Error("Restrict ignored its configured error")
	}
	if len(rec.Calls) != 4 {
		t.Errorf("calls = %v; want four", rec.Calls)
	}
}

func TestProcessFakeOutcomes(t *testing.T) {
	boom := errors.New("boom")
	p := &Process{Rec: &Recorder{}, Bare: map[string]string{"x": "/bin/x"}, LookupErr: boom, ExecErr: boom}
	if path, err := p.Lookup("x"); path != "" || err != boom {
		t.Errorf("Lookup with an error = %q, %v", path, err)
	}
	if err := p.Exec("/bin/x", nil); err != boom {
		t.Errorf("Exec with an error = %v", err)
	}
	p.LookupErr = nil
	if path, err := p.Lookup("./y/z"); path != "./y/z" || err != nil {
		t.Errorf("Lookup of a slash path = %q, %v; want it unchanged", path, err)
	}
	if path, err := p.Lookup("absent"); path != "" || !errors.Is(err, port.ErrNotFound) {
		t.Errorf("Lookup of an absent name = %q, %v", path, err)
	}
	if p.ExecPath != "/bin/x" || len(p.Looked) != 3 {
		t.Errorf("recorded %q after looking up %q", p.ExecPath, p.Looked)
	}
}
