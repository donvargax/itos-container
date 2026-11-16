//go:build !linux

package disk

import "errors"

// Unsupported platforms refuse staging before any file is opened.
const credentialNoFollow = 0
const credentialDirectory = 0

func credentialFilesystem(string) (int64, error) {
	return 0, errors.New("unsupported filesystem observation")
}
