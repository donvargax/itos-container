//go:build !linux

package features

import (
	"errors"
	"os"
)

func lifecyclePipeSize(*os.File) (int, int, error, error) {
	return 0, 0, errors.New("pipe-capacity inspection requires Linux"), nil
}
