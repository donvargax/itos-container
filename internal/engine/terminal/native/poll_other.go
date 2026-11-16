//go:build !linux

package native

import "syscall"

// Only the unavailable poll call lives here. Acquire refuses before any I/O.
const (
	supported    = false
	windowSignal = syscall.Signal(28)
	pollHangup   = 0x10
	pollError    = 0x08
	pollInvalid  = 0x20
)

type pollDescriptor struct {
	Fd              int32
	Events, Revents int16
}

func poll([]pollDescriptor) (int, error) { return 0, syscall.ENOSYS }
