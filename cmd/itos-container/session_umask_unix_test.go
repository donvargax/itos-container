//go:build !windows

package main

import "syscall"

func sessionContractUmask(mask int) func() {
	previous := syscall.Umask(mask)
	return func() { syscall.Umask(previous) }
}
