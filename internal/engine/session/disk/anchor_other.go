//go:build !linux

package disk

import (
	"errors"
	"os"
)

func openPath(*os.Root) (*os.File, error) {
	return nil, errors.ErrUnsupported
}
