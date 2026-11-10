//go:build linux

package features

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lifecyclePipeSize(file *os.File) (int, int, error, error) {
	// File.Fd switches the pipe to blocking mode, defeating the later drain
	// deadline even though SetReadDeadline can still return nil.
	raw, err := file.SyscallConn()
	if err != nil {
		return 0, 0, err, nil
	}
	var capacity, available int
	var capacityErr, readErr error
	err = raw.Control(func(fd uintptr) {
		capacity, capacityErr = unix.FcntlInt(fd, unix.F_GETPIPE_SZ, 0)
		available, readErr = unix.IoctlGetInt(int(fd), unix.TIOCINQ)
	})
	return capacity, available, errors.Join(err, capacityErr), readErr
}
