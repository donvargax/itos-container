package landlock

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// realCalls are the Linux system calls. Landlock has no helper in
// golang.org/x/sys, so the three calls are made here, on the caller's thread:
// a Landlock domain and no_new_privs belong to one thread.
func realCalls() calls {
	return calls{
		handledFS:  unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX,
		noNewPrivs: unix.PR_SET_NO_NEW_PRIVS,
		version: func() (int, error) {
			r, _, e := syscall.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
			return int(r), errno(e) //nolint:gosec // G115: a descriptor or ABI number fits int
		},
		create: func(attr rulesetAttr, flags uint) (int, error) {
			raw := unix.LandlockRulesetAttr{
				Handled_access_fs:  attr.HandledAccessFS,
				Handled_access_net: attr.HandledAccessNet,
				Scoped:             attr.Scoped,
			}
			r, _, e := syscall.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&raw)), unsafe.Sizeof(raw), uintptr(flags)) //nolint:gosec // G103: the kernel reads the attribute through this pointer during the call
			return int(r), errno(e)                                                                                                         //nolint:gosec // G115: a descriptor or ABI number fits int
		},
		prctl: unix.Prctl,
		restrict: func(fd int, flags uint) error {
			_, _, e := syscall.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(fd), uintptr(flags), 0) //nolint:gosec // a descriptor number fits uintptr
			return errno(e)
		},
		close: unix.Close,
	}
}

// errno turns a raw call's errno into an error: nil for zero.
func errno(e syscall.Errno) error {
	if e != 0 {
		return e
	}
	return nil
}
