//go:build unix

package disk

import (
	"os"
	"syscall"
)

func openCandidate(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
