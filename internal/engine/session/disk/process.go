package disk

import (
	"errors"
	"os"
	"syscall"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (w *Workspace) HoldProcess(pid int) (port.Process, port.Outcome) {
	w.mountError = port.MountSystemError{}
	if pid <= 0 {
		return nil, port.MountRefused
	}
	if w.ProcessOpen != nil {
		return w.ProcessOpen(pid)
	}
	fd, err := openPIDFD(pid)
	if err != nil {
		w.mountError = mountSystemError(err)
		return nil, port.MountRefused
	}
	return &workerProcess{file: os.NewFile(uintptr(fd), "worker pidfd"), signal: signalPIDFD}, ""
}

type workerProcess struct {
	mountError port.MountSystemError
	file       *os.File
	signal     func(int) error
}

func (p *workerProcess) Exited() (bool, port.Outcome) {
	p.mountError = port.MountSystemError{}
	err := p.signal(int(p.file.Fd()))
	if errors.Is(err, syscall.ESRCH) {
		return true, ""
	}
	if err != nil {
		p.mountError = mountSystemError(err)
		return false, port.MountRefused
	}
	return false, ""
}

func (p *workerProcess) Close() { _ = p.file.Close() }
