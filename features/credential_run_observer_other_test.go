//go:build !linux

package features

var credentialWatchSystem = credentialWatchIO{open: credentialWatchUnsupported}

func credentialWatchUnsupported() (int, error) {
	return -1, errCredentialRunObserverUnsupported
}

func credentialRunDevice(_ string) (uint64, error) {
	return 0, errCredentialRunObserverUnsupported
}
