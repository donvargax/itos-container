package features

import (
	"flag"
	"os"
	"runtime"
	"strconv"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Only the disposable child changes its umask and syscall availability. Exec
// inherits this thread's filter, as do all threads subsequently made by Go.
func TestApprovalKernelHelper(t *testing.T) {
	args := flag.Args()
	if len(args) == 0 || args[0] != approvalKernelHelper {
		return
	}
	if len(args) < 3 {
		t.Fatal("missing compatibility fixture arguments")
	}
	mask, err := strconv.ParseUint(args[1], 8, 9)
	if err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	// Do not unlock: neither a failure nor exec may move work to another thread.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0}, // seccomp_data.nr
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_FCHMODAT2, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.ENOSYS)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, 0, uintptr(unsafe.Pointer(&program)))
	runtime.KeepAlive(filter)
	if errno != 0 {
		t.Fatal(errno)
	}
	_, _, errno = unix.RawSyscall6(unix.SYS_FCHMODAT2, 0, 0, 0, 0, 0, 0)
	if errno != unix.ENOSYS {
		t.Fatalf("fchmodat2 availability fixture returned %v, want ENOSYS", errno)
	}
	unix.Umask(int(mask))
	if err := unix.Exec(args[2], args[2:], os.Environ()); err != nil {
		t.Fatal(err)
	}
}
