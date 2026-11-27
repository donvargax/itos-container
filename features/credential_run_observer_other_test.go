//go:build !linux

package features

func newCredentialRunObserver(_, _ string) (credentialRunObserver, error) {
	return nil, errCredentialRunObserverUnsupported
}

func credentialRunSameDevice(_, _ string) (bool, error) {
	return false, errCredentialRunObserverUnsupported
}

func verifyCredentialRunFilesystem(_ []string) error {
	return errCredentialRunObserverUnsupported
}

func newCredentialRunStagingObserver() (credentialRunObserver, error) {
	return nil, errCredentialRunObserverUnsupported
}
