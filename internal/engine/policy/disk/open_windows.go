package disk

import (
	"os"
	"syscall"
)

// Open the reparse point itself so Read's handle-based Stat rejects links.
const policyOpenFlags = os.O_RDONLY | syscall.FILE_FLAG_OPEN_REPARSE_POINT

func openPolicy() (*os.File, error) {
	return os.OpenFile("itos-container.yaml", policyOpenFlags, 0)
}
