//go:build !linux

package disk

import "errors"

func credentialFilesystem(string) (int64, error) {
	return 0, errors.New("unsupported filesystem observation")
}
