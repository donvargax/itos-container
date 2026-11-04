//go:build !windows

package features

import "golang.org/x/sys/unix"

func manifestFIFO(path string) error { return unix.Mkfifo(path, 0o600) }
