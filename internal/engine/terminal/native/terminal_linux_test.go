package native

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
	"github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
)

// Only the bounded child signals itself. No test sends a signal to a shell,
// controlling terminal, developer process, or sibling test process.
func TestNativeContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestTerminalChild$", "-test.v"}
	// Go passes this destination as a flag, not necessarily GOCOVERDIR. Keep
	// the child's executed counters in the parent's CI coverage collection.
	if coverdir := flag.Lookup("test.gocoverdir").Value.String(); coverdir != "" {
		args = append(args, "-test.gocoverdir="+coverdir)
	}
	command := exec.CommandContext(ctx, os.Args[0], args...)
	command.Env = append(os.Environ(), "T56_TERMINAL_CHILD=1")
	// Bind a disposable terminal to descriptor zero in the child without
	// replacing or changing the parent process's standard input.
	p := openPair(t)
	command.Stdin = p.slave
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("terminal child: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}

func TestPairRejectsInvalidOutputBeforeChangingInput(t *testing.T) {
	for _, fault := range []string{"missing", "zero file", "closed input", "closed output", "pipe"} {
		t.Run(fault, func(t *testing.T) {
			in, out := openPair(t), openPair(t)
			before, err := term.GetState(int(in.slave.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			p := NewPair(in.slave, out.slave, "linux")
			switch fault {
			case "missing":
				p.Output = nil
			case "zero file":
				p.Output = &os.File{}
			case "closed input":
				p.file, err = os.OpenFile(in.slave.Name(), os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				_ = p.file.Close()
			case "closed output":
				_ = p.Output.Close()
			case "pipe":
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = r.Close() }()
				defer func() { _ = w.Close() }()
				p.Output = w
			}
			lease, err := p.Acquire(t.Context())
			var failure *port.Failure
			if lease != nil || !errors.As(err, &failure) || failure.Kind != port.InvalidTerminal {
				t.Fatalf("invalid terminal accepted: %v %v", lease, err)
			}
			after, err := term.GetState(int(in.slave.Fd()))
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("input changed during output refusal", err)
			}
		})
	}
}

func TestTerminalChild(t *testing.T) {
	if os.Getenv("T56_TERMINAL_CHILD") != "1" {
		return
	}
	t.Setenv("COLUMNS", "500")
	t.Setenv("LINES", "200")
	t.Run("contract", func(t *testing.T) { porttest.Contract(t, terminalFixture) })
	t.Run("controlling_aliases", controllingAliases)
	for _, layout := range []string{"shared", "distinct input", "distinct output"} {
		t.Run(layout, func(t *testing.T) { porttest.Contract(t, pairFixture(layout)) })
	}
	t.Run("invalid_files", invalidFiles)
	t.Run("raw_control_bytes_and_handles", rawControlBytes)
	t.Run("resource_cycles", resourceCycles)
	t.Run("nonblocking_descriptor", nonblockingDescriptor)
	t.Run("standard_input_terminal", standardInputTerminal)
	t.Run("observer_os_failures", observerOSFailures)
	t.Run("busy_during_acquisition", busyDuringAcquisition)
	t.Run("poll_system_error", pollSystemError)
	t.Run("cancel_before_raw", cancelBeforeRaw)
	t.Run("interrupted_observation", interruptedObservation)
	t.Run("interrupted_check", interruptedCheck)
}

func pairFixture(layout string) porttest.Factory {
	return func(t *testing.T, invalid port.FailureKind) porttest.Fixture {
		input := terminalFixture(t, invalid)
		provider := input.Provider.(*Provider)
		if layout == "shared" {
			// A second descriptor for the same device must restore only once.
			p := provider.file
			if p != nil {
				out, err := os.OpenFile(p.Name(), os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = out.Close() })
				provider.Output = out
			}
			return input
		}
		p := openPair(t)
		p.size(t, port.Dimensions{}) // output dimensions never gate acquisition
		provider.Output = p.slave
		beforeSnapshot, beforeChange, beforeRaw := input.Snapshot, input.ChangeState, input.Raw
		input.Snapshot = func() any { return [2]any{beforeSnapshot(), p.snapshot(t)} }
		input.ChangeState = func() { beforeChange(); p.changeState(t) }
		input.Raw = func() bool {
			state, err := unix.IoctlGetTermios(int(p.slave.Fd()), unix.TCGETS)
			return err == nil && state.Oflag&unix.OPOST == 0 && beforeRaw()
		}
		if layout == "distinct output" {
			input.Hangup = func(unread bool) {
				if unread {
					if _, err := p.master.Write([]byte{'x'}); err != nil {
						t.Fatal(err)
					}
				}
				if err := p.master.Close(); err != nil {
					t.Fatal(err)
				}
			}
			input.Break = func() {
				if err := p.slave.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
		return input
	}
}

// Cancel at the second acquisition checkpoint, before MakeRaw, without a
// scheduling window or a signal sent to any process.
type cancelOnRecheck struct {
	context.Context
	cancel  context.CancelFunc
	checked bool
}

func (c *cancelOnRecheck) Err() error {
	if c.checked {
		c.cancel()
	}
	c.checked = true
	return c.Context.Err()
}

func cancelBeforeRaw(t *testing.T) {
	p := openPair(t)
	before := p.snapshot(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := New(p.slave, "linux")
	l, err := provider.Acquire(&cancelOnRecheck{Context: ctx, cancel: cancel})
	if l != nil {
		defer l.Close()
	}
	var failure *port.Failure
	if l != nil || !errors.As(err, &failure) || failure.Kind != port.Cancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel before raw = %v, %v", l, err)
	}
	if !reflect.DeepEqual(before, p.snapshot(t)) {
		t.Fatal("cancelled acquisition changed terminal state")
	}
	// Refusal must also release the provider for its next acquisition.
	l, err = provider.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r := l.Close(); r.Cause != port.Closed || r.RestoreErr != nil {
		t.Fatalf("reacquire after cancellation: %+v", r)
	}
}

// A stateful RawConn fixture starts interrupted, then uses the real PTY for
// every subsequent control operation, including exact-state restoration.
type interruptedConn struct {
	syscall.RawConn
	interrupted bool
}

func (c *interruptedConn) Control(call func(uintptr)) error {
	if !c.interrupted {
		c.interrupted = true
		return syscall.EINTR
	}
	return c.RawConn.Control(call)
}

func interruptedObservation(t *testing.T) {
	p := openPair(t)
	before := p.snapshot(t)
	conn, err := p.slave.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	original, err := term.MakeRaw(int(p.slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	l := &lease{provider: New(p.slave, "linux"), conn: &interruptedConn{RawConn: conn},
		original: original, initial: port.Dimensions{Columns: 93, Rows: 31},
		resizes: make(chan port.Dimensions, 1), done: make(chan struct{}),
		stop: make(chan struct{}), signals: make(chan os.Signal, 1)}
	want := port.Dimensions{Columns: 107, Rows: 43}
	p.size(t, want)
	l.signals <- windowSignal
	go l.run(context.Background())
	defer l.Close()
	select {
	case got, ok := <-l.Resizes():
		if !ok || got != want {
			t.Fatalf("interrupted resize lost: %v, open=%v", got, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("interrupted observation never retried the pending resize")
	}
	if r := l.Close(); r.Cause != port.Closed || r.Err != nil || r.RestoreErr != nil {
		t.Fatalf("interruption ended the lease: %+v", r)
	}
	if !reflect.DeepEqual(before, p.snapshot(t)) {
		t.Fatal("interrupted observation did not restore terminal state")
	}
}

type terminalPair struct{ master, slave *os.File }

func openPair(t *testing.T) terminalPair {
	t.Helper()
	m, s, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	} // Linux evidence never skips unavailable PTYs.
	t.Cleanup(func() { _ = m.Close(); _ = s.Close() })
	p := terminalPair{m, s}
	p.size(t, port.Dimensions{Columns: 93, Rows: 31})
	p.changeState(t)
	return p
}

func (p terminalPair) size(t *testing.T, size port.Dimensions) {
	t.Helper()
	if err := pty.Setsize(p.slave, &pty.Winsize{Cols: uint16(size.Columns), Rows: uint16(size.Rows)}); err != nil {
		t.Fatal(err)
	}
}

func winch(t *testing.T) {
	t.Helper()
	if err := unix.Kill(os.Getpid(), unix.SIGWINCH); err != nil {
		t.Fatal(err)
	}
}

func (p terminalPair) snapshot(t *testing.T) any {
	t.Helper()
	state, err := term.GetState(int(p.slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func (p terminalPair) changeState(t *testing.T) {
	t.Helper()
	state, err := unix.IoctlGetTermios(int(p.slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	state.Lflag ^= unix.ECHONL
	state.Cc[unix.VERASE] ^= 1
	if err := unix.IoctlSetTermios(int(p.slave.Fd()), unix.TCSETS, state); err != nil {
		t.Fatal(err)
	}
}

func terminalFixture(t *testing.T, invalid port.FailureKind) porttest.Fixture {
	p := openPair(t)
	name := "linux"
	file := p.slave
	switch invalid {
	case port.InvalidTerminal:
		file = nil
	case port.Unsupported:
		name = "other"
	case port.InvalidDimensions:
		p.size(t, port.Dimensions{Columns: 93, Rows: 0})
	}
	return porttest.Fixture{
		Provider: New(file, name),
		Raw: func() bool {
			state, err := unix.IoctlGetTermios(int(p.slave.Fd()), unix.TCGETS)
			if err != nil {
				t.Fatal(err)
			}
			return state.Lflag&(unix.ICANON|unix.ECHO|unix.ISIG) == 0
		},
		Snapshot:    func() any { return p.snapshot(t) },
		ChangeState: func() { p.changeState(t) },
		Resize:      func(size port.Dimensions) { p.size(t, size); winch(t) },
		Hangup: func(unread bool) {
			if unread {
				if _, err := p.master.Write([]byte{'x'}); err != nil {
					t.Fatal(err)
				}
			}
			if err := p.master.Close(); err != nil {
				t.Fatal(err)
			}
		},
		Break: func() {
			if err := p.slave.Close(); err != nil {
				t.Fatal(err)
			}
		},
	}
}

func invalidFiles(t *testing.T) {
	p := openPair(t)
	if err := p.slave.Close(); err != nil {
		t.Fatal(err)
	}
	nonterminal, err := os.CreateTemp(t.TempDir(), "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nonterminal.Close() }()
	for _, file := range []*os.File{nil, {}, p.slave, nonterminal} {
		l, err := New(file, "linux").Acquire(context.Background())
		var failure *port.Failure
		if l != nil || !errors.As(err, &failure) || failure.Kind != port.InvalidTerminal {
			t.Fatalf("invalid file = %v, %v", l, err)
		}
	}
}

func rawControlBytes(t *testing.T) {
	p := openPair(t)
	before := p.snapshot(t)
	flags, err := unix.FcntlInt(p.slave.Fd(), unix.F_GETFL, 0)
	if err != nil {
		t.Fatal(err)
	}
	l, err := New(p.slave, "linux").Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	state, err := unix.IoctlGetTermios(int(p.slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	if state.Lflag&(unix.ICANON|unix.ECHO|unix.ISIG) != 0 {
		t.Fatal("raw mode retained canonical, echo or signal flags")
	}
	read := make(chan []byte, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		b := make([]byte, 2)
		_, err := io.ReadFull(p.slave, b)
		if err != nil {
			b = nil
		}
		read <- b
	})
	// Closing the peer on test failure bounds the test-only reader as well.
	defer func() { _ = p.master.Close(); wg.Wait() }()
	if _, err := p.master.Write([]byte{3, 4}); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-read:
		if !bytes.Equal(b, []byte{3, 4}) {
			t.Fatalf("control bytes = %v", b)
		}
	case <-time.After(time.Second):
		t.Fatal("raw control bytes were consumed or interpreted")
	}
	select {
	case <-l.Done():
		t.Fatal("raw control bytes ended lease")
	default:
	}
	if r := l.Close(); r.Cause != port.Closed || r.RestoreErr != nil {
		t.Fatalf("close: %+v", r)
	}
	if !reflect.DeepEqual(before, p.snapshot(t)) {
		t.Fatal("full term.State was not restored")
	}
	after, err := unix.FcntlInt(p.slave.Fd(), unix.F_GETFL, 0)
	if err != nil || after != flags {
		t.Fatalf("descriptor flags changed: %d -> %d, %v", flags, after, err)
	}
	if _, err := p.slave.Write([]byte("still open")); err != nil {
		t.Fatal("caller handle closed", err)
	}
	got := make([]byte, len("still open"))
	if _, err := io.ReadFull(p.master, got); err != nil || string(got) != "still open" {
		t.Fatalf("caller handles unusable: %q, %v", got, err)
	}
}

func resourceCycles(t *testing.T) {
	p := openPair(t)
	provider := New(p.slave, "linux")
	otherSubscription := make(chan os.Signal, 1)
	signal.Notify(otherSubscription, unix.SIGWINCH)
	defer signal.Stop(otherSubscription)
	var leases []*lease
	cycle := func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		l, err := provider.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		leases = append(leases, l.(*lease))
		cancel()
		select {
		case <-l.Done():
		case <-time.After(time.Second):
			t.Fatal("cycle leaked observer")
		}
		if r := l.Close(); r.Cause != port.ContextCancelled || r.RestoreErr != nil {
			t.Fatalf("cycle: %+v", r)
		}
	}
	cycle() // initialize Go's process-wide signal dispatcher before the baseline
	baseline := runtime.NumGoroutine()
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for range 40 {
		cycle()
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(fds) {
		t.Fatalf("descriptor growth: %d -> %d", len(fds), len(after))
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if runtime.NumGoroutine() > baseline {
		t.Fatalf("watcher growth: %d -> %d", baseline, runtime.NumGoroutine())
	}
	winch(t)
	select {
	case <-otherSubscription:
	case <-time.After(time.Second):
		t.Fatal("lease cleanup stopped an unrelated signal subscription")
	}
	// Stop synchronizes with signal dispatch, so every old lease would have
	// received this signal if its subscription had leaked.
	signal.Stop(otherSubscription)
	for _, l := range leases {
		select {
		case <-l.signals:
			t.Fatal("released lease retained its signal subscription")
		default:
		}
	}
}

func nonblockingDescriptor(t *testing.T) {
	p := openPair(t)
	fd, err := unix.FcntlInt(p.slave.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		t.Fatal(err)
	}
	// Wrap through os.NewFile after O_NONBLOCK is set, as a caller whose File
	// is integrated with Go's poller. File.Fd would silently clear that flag.
	file := os.NewFile(uintptr(fd), "nonblocking-terminal")
	t.Cleanup(func() { _ = file.Close() })
	l, err := New(file, "linux").Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	check := func() {
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
		if err != nil || flags&unix.O_NONBLOCK == 0 {
			t.Fatalf("caller O_NONBLOCK lost: %d, %v", flags, err)
		}
	}
	check()
	if r := l.Close(); r.RestoreErr != nil {
		t.Fatal(r.RestoreErr)
	}
	check()
}

func standardInputTerminal(t *testing.T) {
	if fd := os.Stdin.Fd(); fd != 0 {
		t.Fatalf("standard input descriptor = %d, want 0", fd)
	}
	l, err := New(os.Stdin, "linux").Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	want := port.Dimensions{Columns: 107, Rows: 43}
	p := terminalPair{slave: os.Stdin}
	p.size(t, want)
	winch(t)
	select {
	case got, ok := <-l.Resizes():
		if !ok {
			t.Fatalf("standard input observation ended: %+v", l.Result())
		}
		if got != want {
			t.Fatalf("standard input resize = %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("standard input terminal resize was not observed")
	}
	if r := l.Close(); r.Cause != port.Closed || r.Err != nil || r.RestoreErr != nil {
		t.Fatalf("standard input close: %+v", r)
	}
}

func observerOSFailures(t *testing.T) {
	for _, fd := range []int{-1, 2147483648} {
		if hungup, err := disconnected(fd); hungup || err == nil {
			t.Fatalf("invalid descriptor = %v, %v", hungup, err)
		}
	}
	p := openPair(t)
	fd := int(p.slave.Fd())
	if err := p.slave.Close(); err != nil {
		t.Fatal(err)
	}
	if hungup, err := disconnected(fd); hungup || err == nil {
		t.Fatalf("closed descriptor poll = %v, %v", hungup, err)
	}
	if _, err := dimensions(fd); err == nil {
		t.Fatal("closed descriptor size succeeded")
	}
}

func busyDuringAcquisition(t *testing.T) {
	p := openPair(t)
	provider := New(p.slave, "linux")
	before := p.snapshot(t)
	provider.mu.Lock()
	l, err := provider.Acquire(context.Background())
	provider.mu.Unlock()
	var failure *port.Failure
	if l != nil || !errors.As(err, &failure) || failure.Kind != port.Busy {
		t.Fatalf("busy acquisition: %v, %v", l, err)
	}
	if !reflect.DeepEqual(before, p.snapshot(t)) {
		t.Fatal("busy acquisition changed settings")
	}
}

func pollSystemError(t *testing.T) {
	p := openPair(t)
	before := p.snapshot(t)
	l, err := New(p.slave, "linux").Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	// In this disposable child only, nfds=1 exceeds the zero soft limit and
	// poll fails with EINVAL. Existing terminal ioctls still work, allowing
	// deterministic proof of monitor-error cleanup without replacing syscalls.
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: 0, Max: limit.Max}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
			t.Fatal(err)
		}
	}()
	select {
	case <-l.Done():
	case <-time.After(time.Second):
		t.Fatal("poll error did not terminate observation")
	}
	r := l.Result()
	if r.Cause != port.ObservationFailed || !errors.Is(r.Err, unix.EINVAL) || r.RestoreErr != nil {
		t.Fatalf("poll error result: %+v", r)
	}
	if !reflect.DeepEqual(before, p.snapshot(t)) {
		t.Fatal("poll failure did not restore original state")
	}
}
