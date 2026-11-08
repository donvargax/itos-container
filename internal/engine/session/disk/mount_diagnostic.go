package disk

import (
	"errors"
	"syscall"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func mountSystemError(err error) port.MountSystemError {
	if err == nil {
		return port.MountSystemError{}
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return port.MountSystemError{Errno: int(errno), Message: errno.Error()}
	}
	return port.MountSystemError{Message: "unclassified system error"}
}

func (w *Workspace) MountError() port.MountSystemError     { return w.mountError }
func (p *workerProcess) MountError() port.MountSystemError { return p.mountError }
