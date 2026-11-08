package disk

import (
	"os"

	"golang.org/x/sys/unix"
)

func openPath(root *os.Root) (*os.File, error) {
	return root.OpenFile(".", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}

func openPIDFD(pid int) (int, error) { return unix.PidfdOpen(pid, 0) }
func pollPIDFD(fd int32) (int16, error) {
	fds := []unix.PollFd{{Fd: fd, Events: unix.POLLIN}}
	_, err := unix.Poll(fds, 0)
	return fds[0].Revents, err
}
