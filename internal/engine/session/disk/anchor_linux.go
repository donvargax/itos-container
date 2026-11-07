package disk

import (
	"os"

	"golang.org/x/sys/unix"
)

func openPath(root *os.Root) (*os.File, error) {
	return root.OpenFile(".", unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}
