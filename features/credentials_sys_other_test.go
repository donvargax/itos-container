//go:build !linux

package features

import "errors"

func credentialWatchOpen() (int, error) {
	return -1, errors.New("credential observation requires Linux")
}
func credentialWatchAdd(int, string) (int, error) {
	return -1, errors.New("credential observation requires Linux")
}
func credentialWatchClose(int) {}
func credentialWatchRead(int) ([]byte, error) {
	return nil, errors.New("credential observation requires Linux")
}
