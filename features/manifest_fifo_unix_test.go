//go:build unix

package features

import "syscall"

func manifestFIFO(path string) error { return syscall.Mkfifo(path, 0o600) }
