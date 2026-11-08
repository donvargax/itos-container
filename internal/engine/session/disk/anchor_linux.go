package disk

import (
	"os"

	"golang.org/x/sys/unix"
)

func openPath(root *os.Root) (*os.File, error) {
	return root.OpenFile(".", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}

func openPIDFD(pid int) (int, error) { return unix.PidfdOpen(pid, 0) }
func signalPIDFD(fd int) error       { return unix.PidfdSendSignal(fd, 0, nil, 0) }
