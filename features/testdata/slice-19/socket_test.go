package main

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// SO_RCVTIMEO makes a blocked receive interruptible even with the Go signal
// handler's SA_RESTART. SIGURG is handled by the runtime. Keep this goroutine
// on the addressed thread until the sender finishes, including on failure.
func interruptedReceive(t *testing.T, kind int, receive func(int) (string, error)) (string, error) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, kind|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	if err = unix.SetsockoptTimeval(fds[0], unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 1}); err != nil {
		t.Fatal(err)
	}
	// A stream prefix must survive an interruption of the next receive.
	payload := "slice-19-nonce\n"
	if kind == unix.SOCK_STREAM {
		if _, err = unix.Write(fds[1], []byte("slice-19-")); err != nil {
			t.Fatal(err)
		}
		payload = "nonce\n"
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tid := unix.Gettid()
	done := make(chan error, 1)
	go func() {
		// Repeated signals span entry into the blocking syscall. The raw
		// receive control below must observe EINTR, proving the stimulus.
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for range 50 {
			<-ticker.C
			if e := unix.Tgkill(unix.Getpid(), tid, unix.SIGURG); e != nil {
				done <- e
				return
			}
		}
		_, e := unix.Write(fds[1], []byte(payload))
		done <- e
	}()
	value, err := receive(fds[0])
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	return value, err
}

func TestMessageRetriesInterruptions(t *testing.T) {
	t.Run("raw receive observes EINTR", func(t *testing.T) {
		_, err := interruptedReceive(t, unix.SOCK_SEQPACKET, func(fd int) (string, error) {
			b := make([]byte, 128)
			_, _, err := unix.Recvfrom(fd, b, 0)
			return "", err
		})
		if !errors.Is(err, unix.EINTR) {
			t.Fatalf("signal control did not interrupt receive: %v", err)
		}
	})
	for name, kind := range map[string]int{"stream": unix.SOCK_STREAM, "seqpacket": unix.SOCK_SEQPACKET, "dgram": unix.SOCK_DGRAM} {
		t.Run(name, func(t *testing.T) {
			value, err := interruptedReceive(t, kind, func(fd int) (string, error) {
				value, _, err := message(fd, kind)
				return value, err
			})
			if err != nil || value != "slice-19-nonce" {
				t.Fatalf("interrupted exchange lost its nonce: %q %v", value, err)
			}
		})
	}
}

func TestMessagePreservesReceiveFailures(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	if err = unix.SetsockoptTimeval(fds[0], unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Usec: 20000}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = message(fds[0], unix.SOCK_SEQPACKET); !errors.Is(err, unix.EAGAIN) {
		t.Fatalf("receive timeout was lost: %v", err)
	}
	if _, _, err = message(-1, unix.SOCK_SEQPACKET); !errors.Is(err, unix.EBADF) {
		t.Fatalf("invalid descriptor failure was lost: %v", err)
	}
}
