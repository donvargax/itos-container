package landlock

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port/porttest"
)

const childEnv = "T65_LANDLOCK_CHILD"

// rulesetLink is the exact readlink target Linux gives a ruleset descriptor.
const rulesetLink = "anon_inode:[landlock-ruleset]"

// testRealAdapter runs the contract against the real adapter in a child of
// this test binary: Restrict cannot be undone, so the test process itself is
// never restricted.
func testRealAdapter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestLandlockChild$", "-test.v"}
	// Go passes this destination as a flag, not necessarily GOCOVERDIR. Keep
	// the child's executed counters in the parent's coverage collection.
	if dir := flag.Lookup("test.gocoverdir").Value.String(); dir != "" {
		args = append(args, "-test.gocoverdir="+dir)
	}
	command := exec.CommandContext(ctx, os.Args[0], args...)
	command.Env = append(os.Environ(), childEnv+"=1")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("landlock child: %v\n%s", err, out)
	}
}

// measureABI is an independent measurement: its own raw call, not the adapter's.
func measureABI() int {
	r, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if e != 0 {
		return 0
	}
	return int(r)
}

// rulesetFDs lists the descriptors of this process that link to a ruleset.
func rulesetFDs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("reading /proc/self/fd: %v", err)
	}
	var found []string
	for _, entry := range entries {
		if target, err := os.Readlink("/proc/self/fd/" + entry.Name()); err == nil && target == rulesetLink {
			found = append(found, entry.Name())
		}
	}
	return found
}

// unixSocket makes a socket of kind bound to path, listening when it streams.
func unixSocket(t *testing.T, kind int, path string) int {
	t.Helper()
	fd, err := unix.Socket(unix.AF_UNIX, kind|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err != nil {
		t.Fatalf("bind %s: %v", path, err)
	}
	if kind == unix.SOCK_STREAM {
		if err := unix.Listen(fd, 4); err != nil {
			t.Fatalf("listen: %v", err)
		}
	}
	return fd
}

// connectTo and sendTo return the error of one stream connect and one
// datagram send to path, each from a fresh socket.
func connectTo(t *testing.T, path string) error {
	t.Helper()
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	defer func() { _ = unix.Close(fd) }()
	return unix.Connect(fd, &unix.SockaddrUnix{Name: path})
}

func sendTo(t *testing.T, path string) error {
	t.Helper()
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	defer func() { _ = unix.Close(fd) }()
	return unix.Sendto(fd, []byte("x"), 0, &unix.SockaddrUnix{Name: path})
}

// TestLandlockChild is the real adapter's side of testRealAdapter. Run on its
// own it does nothing. Its goroutine locks its thread before its first
// Landlock call and never unlocks, and every call and probe runs on it: a
// Landlock domain belongs to one thread.
func TestLandlockChild(t *testing.T) {
	if os.Getenv(childEnv) == "" {
		return
	}
	runtime.LockOSThread()
	abi := measureABI()
	dir := socketDir(t)
	defer func() { _ = os.RemoveAll(dir) }()
	streamPath, dgramPath, ownPath := dir+"/stream", dir+"/dgram", dir+"/own"

	porttest.LandlockContract(t, func() (port.Landlock, porttest.LandlockWant) {
		if abi >= 1 {
			// The positive control: the probe must see a ruleset descriptor
			// this child opened itself, before its absence means anything.
			attr := unix.LandlockRulesetAttr{Handled_access_fs: unix.LANDLOCK_ACCESS_FS_EXECUTE}
			fd, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(uintptr(unsafe.Pointer(&attr))), unsafe.Sizeof(attr), 0)
			if e != 0 {
				t.Fatalf("control ruleset: %v", e)
			}
			if got := rulesetFDs(t); len(got) != 1 {
				t.Errorf("probe saw ruleset descriptors %v with one open; want exactly one", got)
			}
			if err := unix.Close(int(fd)); err != nil {
				t.Fatal(err)
			}
		}
		if abi >= porttest.MinABI {
			unixSocket(t, unix.SOCK_STREAM, streamPath)
			unixSocket(t, unix.SOCK_DGRAM, dgramPath)
			if err := connectTo(t, streamPath); err != nil {
				t.Errorf("connect before Restrict = %v; want success", err)
			}
			if err := sendTo(t, dgramPath); err != nil {
				t.Errorf("sendto before Restrict = %v; want success", err)
			}
		}
		return New(), porttest.LandlockWant{Unavailable: abi == 0, ABI: abi}
	})

	if got := rulesetFDs(t); len(got) != 0 {
		t.Errorf("after Restrict, descriptors %v link to a ruleset; want none", got)
	}
	if abi < porttest.MinABI {
		return
	}
	if err := connectTo(t, streamPath); !errors.Is(err, syscall.EACCES) {
		t.Errorf("stream connect after Restrict = %v; want EACCES", err)
	}
	if err := sendTo(t, dgramPath); !errors.Is(err, syscall.EACCES) {
		t.Errorf("datagram sendto after Restrict = %v; want EACCES", err)
	}
	unixSocket(t, unix.SOCK_STREAM, ownPath)
	if err := connectTo(t, ownPath); err != nil {
		t.Errorf("connect to a socket made after Restrict = %v; want success", err)
	}
}

// TestRealCalls holds the real raw calls to the kernel's answers without
// restricting this process: bad arguments fail, a valid ruleset opens and
// closes, and the two constants are the kernel header's.
func TestRealCalls(t *testing.T) {
	c := realCalls()
	if c.handledFS != 1<<16 || c.noNewPrivs != unix.PR_SET_NO_NEW_PRIVS {
		t.Errorf("constants = %#x, %d; want 1<<16 and PR_SET_NO_NEW_PRIVS", c.handledFS, c.noNewPrivs)
	}
	abi := measureABI()
	got, err := c.version()
	if abi == 0 {
		if !errors.Is(err, syscall.ENOSYS) && !errors.Is(err, syscall.EOPNOTSUPP) {
			t.Errorf("version() = %d, %v; want ENOSYS or EOPNOTSUPP", got, err)
		}
	} else if err != nil || got != abi {
		t.Errorf("version() = %d, %v; want %d, nil", got, err, abi)
	}
	if err := c.close(-1); !errors.Is(err, syscall.EBADF) {
		t.Errorf("close(-1) = %v; want EBADF", err)
	}
	if err := c.restrict(-1, 0); err == nil {
		t.Error("restrict(-1) = nil; want an error")
	}
	if err := c.prctl(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0); err != nil {
		t.Errorf("prctl(PR_GET_NO_NEW_PRIVS) = %v; want nil", err)
	}
	if abi == 0 {
		return
	}
	if err := c.restrict(-1, 0); !errors.Is(err, syscall.EBADF) {
		t.Errorf("restrict(-1) = %v; want EBADF", err)
	}
	if _, err := c.create(rulesetAttr{}, 0xffff); !errors.Is(err, syscall.EINVAL) {
		t.Errorf("create with unknown flags = %v; want EINVAL", err)
	}
	fd, err := c.create(rulesetAttr{HandledAccessFS: unix.LANDLOCK_ACCESS_FS_EXECUTE}, 0)
	if err != nil || fd < 0 {
		t.Fatalf("create = %d, %v; want a descriptor", fd, err)
	}
	if err := c.close(fd); err != nil {
		t.Errorf("close(%d) = %v", fd, err)
	}
}

// socketDir makes a private directory short enough for a socket path: TMPDIR
// can be longer than the 108 bytes a sockaddr_un holds.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "t65")
	if err == nil && len(dir) > 60 {
		_ = os.Remove(dir)
		dir, err = os.MkdirTemp("/tmp", "t65")
	}
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
