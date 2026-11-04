//go:build unix

package disk

import "syscall"

func makeFIFO(path string) error {
	return syscall.Mkfifo(path, 0600)
}
