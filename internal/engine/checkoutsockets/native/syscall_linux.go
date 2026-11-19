package native

import "golang.org/x/sys/unix"

// The Landlock version query: the system call number and flag come from the
// pinned golang.org/x/sys.
const (
	versionCall uintptr = unix.SYS_LANDLOCK_CREATE_RULESET
	versionFlag uintptr = unix.LANDLOCK_CREATE_RULESET_VERSION
)

var rawSyscall = unix.Syscall
