//go:build !linux

package native

import "syscall"

// Other systems have no Landlock query. The detector refuses their platform
// before any probe; a probe built anyway fails instead of reporting absence.
const (
	versionCall uintptr = 0
	versionFlag uintptr = 0
)

func rawSyscall(uintptr, uintptr, uintptr, uintptr) (uintptr, uintptr, syscall.Errno) {
	return ^uintptr(0), 0, syscall.EPERM
}
