//go:build !linux

package disk

import "errors"

func openPrivate(uintptr, string) (int, error) { return -1, errors.ErrUnsupported }
