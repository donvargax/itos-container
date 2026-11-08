package disk

import (
	"errors"
	"os"
	"syscall"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (w *Workspace) HoldProcess(pid int) (port.Process, port.Outcome) {
	if pid <= 0 {
		return nil, port.MountRefused
	}
	if w.ProcessOpen != nil {
		return w.ProcessOpen(pid)
	}
	fd, err := openPIDFD(pid)
	if err != nil {
		return nil, port.MountRefused
	}
	return &workerProcess{file: os.NewFile(uintptr(fd), "worker pidfd"), signal: signalPIDFD}, ""
}

type workerProcess struct {
	file   *os.File
	signal func(int) error
}

func (p *workerProcess) Exited() (bool, port.Outcome) {
	err := p.signal(int(p.file.Fd()))
	if errors.Is(err, syscall.ESRCH) {
		return true, ""
	}
	if err != nil {
		return false, port.MountRefused
	}
	return false, ""
}

func (p *workerProcess) Close() { _ = p.file.Close() }
