//go:build unix

package features

import "golang.org/x/sys/unix"

func approvalFIFO(path string) error { return unix.Mkfifo(path, 0600) }
