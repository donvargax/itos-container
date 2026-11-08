package porttest

import "github.com/donvargax/itos-container/internal/engine/session/port"

// Process models a held process independently from numeric PID observations.
type Process struct {
	Ended, Closed bool
	Failure       port.Outcome
}

func (p *Process) Exited() (bool, port.Outcome) {
	if p.Closed {
		return false, port.MountRefused
	}
	return p.Ended, p.Failure
}
func (p *Process) Close() { p.Closed = true }

func (w *Workspace) HoldProcess(pid int) (port.Process, port.Outcome) {
	if pid <= 0 {
		return nil, port.MountRefused
	}
	if w.AbsentProcesses[pid] {
		return nil, port.ProcessAbsent
	}
	return &Process{}, ""
}

// ProcessAbsenceContract distinguishes a missing process from other failures
// without supplying a handle or claiming the container has exited.
func ProcessAbsenceContract(t Reporter, w port.ProcessWorkspace, pid int) {
	t.Helper()
	for range 2 {
		if p, failure := w.HoldProcess(pid); p != nil || failure != port.ProcessAbsent {
			t.Errorf("absent process acquisition: %v %s", p, failure)
		}
	}
}

// ProcessContract checks that the same reference observes exit and that a
// closed reference cannot authorize an observation of a recycled numeric PID.
func ProcessContract(t Reporter, p port.Process, exit func()) {
	t.Helper()
	if ended, failure := p.Exited(); failure != "" || ended {
		t.Errorf("live process: %t %s", ended, failure)
	}
	exit()
	if ended, failure := p.Exited(); failure != "" || !ended {
		t.Errorf("exited process: %t %s", ended, failure)
	}
	p.Close()
	if _, failure := p.Exited(); failure != port.MountRefused {
		t.Errorf("closed process supplied authority: %s", failure)
	}
	p.Close()
}
