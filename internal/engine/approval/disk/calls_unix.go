//go:build unix

package disk

import (
	"golang.org/x/sys/unix"
	"os"
)

const noFollow = unix.O_NOFOLLOW | unix.O_NONBLOCK

func unlock(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_UN) }

func tryLock(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

func trySharedLock(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_SH|unix.LOCK_NB) }
