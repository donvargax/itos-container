//go:build unix

package disk

import (
	"os"
	"syscall"
)

// Nonblocking open lets Read reject a FIFO without waiting for a writer.
const policyOpenFlags = os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK

func openPolicy() (*os.File, error) {
	return os.OpenFile("itos-container.yaml", policyOpenFlags, 0)
}
