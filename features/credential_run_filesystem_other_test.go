//go:build !linux

package features

import "io"

var credentialRunFilesystemSystem = credentialRunFilesystemIO{openMounts: credentialRunMountsUnsupported}

func credentialRunMountsUnsupported() (io.ReadCloser, error) {
	return nil, errCredentialRunObserverUnsupported
}
