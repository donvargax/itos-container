package disk

import (
	"errors"
	"os"
	"syscall"
)

const noFollow = syscall.FILE_FLAG_OPEN_REPARSE_POINT

func lock(*os.File, bool) error { return errors.New("approval storage requires Unix locking") }
func unlock(*os.File) error     { return nil }

func tryLock(*os.File) error { return errors.New("protected storage requires Unix locking") }
