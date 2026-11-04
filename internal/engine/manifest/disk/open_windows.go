package disk

import (
	"os"
	"syscall"
)

func openCandidate(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}
