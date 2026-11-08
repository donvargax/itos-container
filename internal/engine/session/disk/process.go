package disk

import (
	"errors"
	"math"
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
		if errors.Is(err, syscall.ESRCH) {
			return nil, port.ProcessAbsent
		}
		return nil, port.MountRefused
	}
	return &workerProcess{file: os.NewFile(uintptr(fd), "worker pidfd"), poll: pollPIDFD}, ""
}

type workerProcess struct {
	mountError port.MountSystemError
	file       *os.File
	poll       func(int32) (int16, error)
}

// Linux poll event bits. Keep interpretation outside the OS-specific syscall
// leaf so invalid references cannot masquerade as process exit readiness.
const (
	pidfdReadable = 0x0001
	pidfdHangup   = 0x0010
)

func (p *workerProcess) Exited() (bool, port.Outcome) {
	p.mountError = port.MountSystemError{}
	fd := p.file.Fd()
	if fd > math.MaxInt32 {
		p.mountError = mountSystemError(syscall.EBADF)
		return false, port.MountRefused
	}
	// Signal 0 also succeeds for zombies, whose /proc root is already gone.
	// Readability observes exit before reaping without consuming its wait status.
	events, err := p.poll(int32(fd))
	if err != nil {
		p.mountError = mountSystemError(err)
		return false, port.MountRefused
	}
	switch events {
	case 0:
		return false, ""
	case pidfdReadable, pidfdReadable | pidfdHangup:
		return true, ""
	default:
		p.mountError = port.MountSystemError{Message: "unexpected pidfd poll events"}
		return false, port.MountRefused
	}
}

func (p *workerProcess) Close() { _ = p.file.Close() }
