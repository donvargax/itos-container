package native

import "golang.org/x/sys/unix"

const (
	supported    = true
	windowSignal = unix.SIGWINCH
	pollHangup   = unix.POLLHUP
	pollError    = unix.POLLERR
	pollInvalid  = unix.POLLNVAL
)

type pollDescriptor = unix.PollFd

func poll(descriptors []pollDescriptor) (int, error) { return unix.Poll(descriptors, 0) }

func terminalDevice(fd int) (int, error) { return unix.IoctlGetInt(fd, unix.TIOCGDEV) }
