package native

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"sync"
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
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTerminalChild$", "-test.v")
	command.Env = append(os.Environ(), "T56_TERMINAL_CHILD=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("terminal child: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}

func TestTerminalChild(t *testing.T) {
	if os.Getenv("T56_TERMINAL_CHILD") != "1" {
		return
	}
	t.Run("contract", func(t *testing.T) { porttest.Contract(t, terminalFixture) })
	t.Run("invalid_files", invalidFiles)
	t.Run("raw_control_bytes_and_handles", rawControlBytes)
	t.Run("resource_cycles", resourceCycles)
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
	for _, file := range []*os.File{nil, p.slave, nonterminal} {
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
	cycle := func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		l, err := provider.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
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
}
