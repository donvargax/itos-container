//go:build linux

package features

import (
	"os"

	"golang.org/x/sys/unix"
)

func lifecyclePipeSize(file *os.File) (int, int, error, error) {
	capacity, err := unix.FcntlInt(file.Fd(), unix.F_GETPIPE_SZ, 0)
	available, readErr := unix.IoctlGetInt(int(file.Fd()), unix.TIOCINQ)
	return capacity, available, err, readErr
}
