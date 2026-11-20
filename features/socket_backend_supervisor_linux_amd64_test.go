package features

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// landlockHelperMode is the mode of the features binary that installs the
// seccomp filter and execs the program. Its arguments are the descriptor of its
// own socket, the umask in octal or "-", the program's path and its argv.
const landlockHelperMode = "itos-container-landlock-helper"

// The layouts of the kernel's seccomp_notif, seccomp_notif_resp and
// seccomp_data. x/sys has their constants and ioctl numbers but not these
// types. libseccomp-golang would add them, but it needs cgo and libseccomp's
// headers, and the filter is a few BPF instructions.
type seccompData struct {
	Nr                 int32
	Arch               uint32
	InstructionPointer uint64
	Args               [6]uint64
}

type seccompNotif struct {
	ID    uint64
	Pid   uint32
	Flags uint32
	Data  seccompData
}

type seccompNotifResp struct {
	ID    uint64
	Val   int64
	Error int32
	Flags uint32
}

// landlockLayout is the one check that these layouts are the kernel's: a larger
// kernel struct would overrun the buffer of a receive.
var landlockLayout = sync.OnceValue(func() error {
	var sizes struct{ Notif, NotifResp, Data uint16 }
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_GET_NOTIF_SIZES, 0, uintptr(unsafe.Pointer(&sizes)))
	if errno != 0 {
		return fmt.Errorf("socket backend fixture: read the seccomp notification sizes: %w", errno)
	}
	if int(sizes.Notif) != int(unsafe.Sizeof(seccompNotif{})) || int(sizes.NotifResp) != int(unsafe.Sizeof(seccompNotifResp{})) || int(sizes.Data) != int(unsafe.Sizeof(seccompData{})) {
		return fmt.Errorf("socket backend fixture: the kernel's seccomp notification sizes %+v are not the fixture's layouts", sizes)
	}
	return nil
})

// landlockHelperError is the helper's own report that it could not install its
// filter or exec the program. The program never ran.
type landlockHelperError struct{ text string }

func (e *landlockHelperError) Error() string { return "socket backend fixture: helper: " + e.text }

// landlockRun is the supervision of one invocation: its listener, its record
// and what went wrong. A route applies the wrapper to the exec.Cmd it builds
// before Start, and ends it after Wait.
type landlockRun struct {
	state      *landlockWorld
	invocation *landlockInvocation
	fd         int      // the harness's end of the helper's socket
	child      *os.File // the helper's end, which only the helper may hold
	received   chan struct{}
	supervised chan struct{} // nil until the helper delivers its listener
	listener   int
	stop       int // an eventfd that ends the answering loop

	mu   sync.Mutex
	errs []error

	once   sync.Once
	result error
}

// superviseLandlock applies the wrapper to cmd, which the route then starts. A
// world that pins nothing starts its program directly, and gets no supervision.
// umask is set by the helper after its filter and just before its exec, so no
// other program runs between the helper and the program, or landlockNoUmask.
// The program keeps the pid the route started, its path, argv, environment,
// working directory, standard streams, signal sets and the route's own extra
// descriptors at their numbers; none of the helper's descriptors reaches it.
func (w *world) superviseLandlock(cmd *exec.Cmd, umask int) (*landlockRun, error) {
	state := w.landlock
	if state == nil {
		return nil, nil
	}
	if err := landlockLayout(); err != nil {
		return nil, err
	}
	helper, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("socket backend fixture: %w", err)
	}
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("socket backend fixture: %w", err)
	}
	argv := cmd.Args
	if len(argv) == 0 {
		argv = []string{cmd.Path}
	}
	umaskText := "-"
	if umask != landlockNoUmask {
		umaskText = strconv.FormatInt(int64(umask), 8)
	}
	child := os.NewFile(uintptr(pair[1]), "socket-backend-helper")
	// The helper's socket follows the route's own descriptors, at the number
	// os/exec gives it.
	number := 3 + len(cmd.ExtraFiles)
	cmd.Args = append([]string{helper, landlockHelperMode, strconv.Itoa(number), umaskText, cmd.Path}, argv...)
	cmd.Path = helper
	cmd.ExtraFiles = append(cmd.ExtraFiles, child)

	run := &landlockRun{state: state, fd: pair[0], child: child, received: make(chan struct{}), listener: -1, stop: -1}
	run.invocation = state.begin(run)
	go run.receive()
	return run, nil
}

// fail records an error of the fixture, which fails the invocation's step.
func (r *landlockRun) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, err)
}

// receive reads what the helper sends: its listener, then nothing while its
// exec succeeds, or the reason it failed.
func (r *landlockRun) receive() {
	defer close(r.received)
	payload := make([]byte, 4096)
	oob := make([]byte, unix.CmsgSpace(4*4))
	for {
		n, oobn, flags, _, err := unix.Recvmsg(r.fd, payload, oob, unix.MSG_CMSG_CLOEXEC)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			r.fail(fmt.Errorf("socket backend fixture: read the helper's report: %w", err))
			return
		}
		if n == 0 && oobn == 0 {
			return // the helper's exec closed its end
		}
		var fds []int
		if messages, err := unix.ParseSocketControlMessage(oob[:oobn]); err != nil {
			r.fail(fmt.Errorf("socket backend fixture: read the helper's report: %w", err))
		} else {
			for i := range messages {
				if rights, err := unix.ParseUnixRights(&messages[i]); err == nil {
					fds = append(fds, rights...)
				}
			}
		}
		if flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 {
			r.fail(errors.New("socket backend fixture: the helper's report was truncated"))
		}
		for _, fd := range fds {
			r.adopt(fd)
		}
		if text, failed := strings.CutPrefix(string(payload[:n]), "error: "); failed {
			r.fail(&landlockHelperError{text: text})
		}
	}
}

// adopt takes the helper's listener and starts answering it.
func (r *landlockRun) adopt(listener int) {
	if r.supervised != nil {
		_ = unix.Close(listener)
		r.fail(errors.New("socket backend fixture: the helper delivered a second listener"))
		return
	}
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC)
	if err != nil {
		_ = unix.Close(listener)
		r.fail(fmt.Errorf("socket backend fixture: %w", err))
		return
	}
	r.listener, r.stop = listener, stop
	r.supervised = make(chan struct{})
	go r.supervise()
}

// supervise answers each call until the run is ended, or until the listener
// reports POLLHUP: every process under the filter has exited and been reaped.
func (r *landlockRun) supervise() {
	defer close(r.supervised)
	fds := []unix.PollFd{{Fd: int32(r.listener), Events: unix.POLLIN}, {Fd: int32(r.stop), Events: unix.POLLIN}}
	for {
		fds[0].Revents, fds[1].Revents = 0, 0
		if _, err := unix.Poll(fds, -1); err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			r.fail(fmt.Errorf("socket backend fixture: wait for a call: %w", err))
			return
		}
		switch {
		case fds[0].Revents&unix.POLLIN != 0:
			// A call that was received is answered and recorded before the run can end.
			if !r.answer() {
				return
			}
		case fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0, fds[1].Revents != 0:
			return
		}
	}
}

// answer receives one call and answers it, reporting whether to go on.
func (r *landlockRun) answer() bool {
	var request seccompNotif
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(r.listener), unix.SECCOMP_IOCTL_NOTIF_RECV, uintptr(unsafe.Pointer(&request))); errno != 0 {
		if errno == unix.ENOENT || errno == unix.EINTR {
			return true // the call is gone: interrupted or ended before it was received
		}
		r.fail(fmt.Errorf("socket backend fixture: receive a call: %w", errno))
		return false
	}
	landlock := request.Data.Arch == unix.AUDIT_ARCH_X86_64 && request.Data.Nr == unix.SYS_LANDLOCK_CREATE_RULESET
	if !landlock {
		// The filter notifies nothing else. Answer, so the program does not hang.
		r.fail(fmt.Errorf("socket backend fixture: the filter notified system call %d of architecture %#x", request.Data.Nr, request.Data.Arch))
		return r.send(seccompNotifResp{ID: request.ID, Error: -int32(unix.EPERM)})
	}
	// landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION)
	version := request.Data.Args[0] == 0 && request.Data.Args[1] == 0 && request.Data.Args[2] == unix.LANDLOCK_CREATE_RULESET_VERSION
	answer := "EPERM"
	if version {
		if action := r.state.takeAction(); action != nil {
			if err := action(); err != nil {
				r.fail(fmt.Errorf("socket backend fixture: the action arranged for the query: %w", err))
			}
		}
		answer = r.state.current()
	}
	response := seccompNotifResp{ID: request.ID}
	abi, errno, err := parseLandlockAnswer(answer)
	switch {
	case err != nil:
		r.fail(err)
		response.Error = -int32(unix.EPERM)
	case errno != "":
		response.Error = -int32(landlockErrnos[errno])
	default:
		response.Val = int64(abi)
	}
	delivered, ok := r.deliver(response)
	if delivered {
		r.state.record(r.invocation, landlockCall{Version: version, Answer: answer})
	}
	return ok
}

// landlockErrnos are the errnos a pinned answer may name.
var landlockErrnos = map[string]unix.Errno{"ENOSYS": unix.ENOSYS, "EOPNOTSUPP": unix.EOPNOTSUPP, "EPERM": unix.EPERM}

// send answers a notification that is not recorded, reporting whether to go on.
func (r *landlockRun) send(response seccompNotifResp) bool {
	_, ok := r.deliver(response)
	return ok
}

// deliver answers a call. delivered says the kernel took the answer, which is
// when a call counts; ok says whether to go on answering.
func (r *landlockRun) deliver(response seccompNotifResp) (delivered, ok bool) {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(r.listener), unix.SECCOMP_IOCTL_NOTIF_SEND, uintptr(unsafe.Pointer(&response)))
	switch errno {
	case 0:
		return true, true
	case unix.ENOENT:
		// Either a signal interrupted the call, which Go's restarting handlers
		// turn into a new notification, or the program ended.
		return false, true
	}
	r.fail(fmt.Errorf("socket backend fixture: answer a call: %w", errno))
	return false, false
}

// end ends the supervision after the route's Wait: it closes the listener once
// each call already received is answered and recorded, so a record read after
// the route returns is complete. started says whether Start succeeded. It
// returns what went wrong, a helper's own report as a *landlockHelperError, and
// is safe to call again.
func (r *landlockRun) end(started bool) error {
	if r == nil {
		return nil
	}
	r.once.Do(func() { r.result = r.finish(started) })
	return r.result
}

func (r *landlockRun) finish(started bool) error {
	_ = r.child.Close()
	select {
	case <-r.received:
	case <-time.After(5 * time.Second):
		// Something other than the helper holds its end of the socket.
		_ = unix.Shutdown(r.fd, unix.SHUT_RDWR)
		<-r.received
	}
	if r.supervised != nil {
		_, _ = unix.Write(r.stop, []byte{1, 0, 0, 0, 0, 0, 0, 0})
		<-r.supervised
		_ = unix.Close(r.listener)
		_ = unix.Close(r.stop)
	}
	_ = unix.Close(r.fd)
	r.state.forget(r)
	r.mu.Lock()
	errs := slices.Clone(r.errs)
	r.mu.Unlock()
	if started && r.supervised == nil && len(errs) == 0 {
		errs = append(errs, errors.New("socket backend fixture: the helper delivered no listener"))
	}
	return errors.Join(errs...)
}

// landlockHelperMain is the helper: it locks its thread, sets no_new_privs and
// installs a filter whose only action but allow is a user notification for
// landlock_create_ruleset, hands the listener to the harness and execs the
// program in the same process. It never runs the program unfiltered, and
// reports a failure on its descriptor, which closes when the exec succeeds.
func landlockHelperMain(args []string) {
	if len(args) < 4 {
		fmt.Fprintln(os.Stderr, "landlock helper: missing arguments")
		os.Exit(70)
	}
	socket, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "landlock helper:", err)
		os.Exit(70)
	}
	// os/exec hands this descriptor over without close-on-exec.
	unix.CloseOnExec(socket)
	report := func(err error) {
		_ = unix.Sendmsg(socket, []byte("error: "+err.Error()), nil, nil, 0)
		os.Exit(70)
	}
	mask := -1
	if args[1] != "-" {
		parsed, err := strconv.ParseUint(args[1], 8, 9)
		if err != nil {
			report(err)
		}
		mask = int(parsed)
	}
	path, argv := args[2], args[3:]

	// Do not unlock: neither a failure nor exec may move work to another
	// thread, and exec inherits this thread's filter.
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		report(fmt.Errorf("set no_new_privs: %w", err))
	}
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 4}, // seccomp_data.arch
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.AUDIT_ARCH_X86_64, Jf: 3},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0}, // seccomp_data.nr
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_LANDLOCK_CREATE_RULESET, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_USER_NOTIF},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	listener, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_NEW_LISTENER, uintptr(unsafe.Pointer(&program)))
	runtime.KeepAlive(filter)
	if errno != 0 {
		report(fmt.Errorf("install the seccomp filter: %w", errno))
	}
	if err := unix.Sendmsg(socket, []byte("listener"), unix.UnixRights(int(listener)), nil, 0); err != nil {
		report(fmt.Errorf("hand over the listener: %w", err))
	}
	if mask >= 0 {
		unix.Umask(mask)
	}
	// Only a failure returns. The listener and the socket close on exec.
	report(fmt.Errorf("execute %s: %w", path, unix.Exec(path, argv, os.Environ())))
}
