//go:build !linux

package features

import "errors"

func sessionInotifyInit() (int, error)            { return 0, errors.New("inotify requires Linux") }
func sessionInotifyAdd(int, string) (int, error)  { return 0, errors.New("inotify requires Linux") }
func sessionInotifyRead(int, []byte) (int, error) { return 0, errors.New("inotify requires Linux") }
