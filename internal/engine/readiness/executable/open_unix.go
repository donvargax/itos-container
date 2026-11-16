//go:build unix

package executable

import (
	"os"
	"syscall"
)

// Nonblocking, so a pipe put where the program was cannot hold the open.
const openFlags = os.O_RDONLY | syscall.O_NONBLOCK
