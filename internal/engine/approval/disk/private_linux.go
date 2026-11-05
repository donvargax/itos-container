package disk

import "golang.org/x/sys/unix"

func openPrivate(parent uintptr, name string) (int, error) {
	return unix.Openat(int(parent), name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
}
