//go:build !linux

package disk

import (
	"errors"
	"os"
)

func openPath(*os.Root) (*os.File, error) {
	return nil, errors.ErrUnsupported
}

func openPIDFD(int) (int, error)     { return -1, errors.ErrUnsupported }
func pollPIDFD(int32) (int16, error) { return 0, errors.ErrUnsupported }
