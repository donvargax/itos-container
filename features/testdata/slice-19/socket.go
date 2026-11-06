// Trusted, synthetic Unix-socket controls. Built explicitly by the feature
// harness; never imported by the product or compiled from a consumer checkout.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

type target struct{ Name, Op, Path string }
type attempt struct {
	Target, Operation, Path, Nonce          string
	Invoked, Connected, Sent                bool
	Errno                                   int
	Error, SetupError, ExchangeError, Reply string
}
type fileRead struct{ Target, Path, Data, Alias, Error string }
type report struct {
	Kind, Nonce, Phase, Process, Target, Error string
	PID, PPID, UID, GID                        int
	Attempts                                   []attempt
	Reads                                      []fileRead
	Receipts                                   []string
	Tmpfs                                      bool
}
type config struct {
	Nonce, Mode, Root, Suffix string
	Targets                   []target
}

var outputMu sync.Mutex

func socketType(op string) int {
	switch op {
	case "stream-connect":
		return unix.SOCK_STREAM
	case "seqpacket-connect":
		return unix.SOCK_SEQPACKET
	case "dgram-connect", "dgram-sendto", "dgram-sendmsg":
		return unix.SOCK_DGRAM
	default:
		panic("unknown fixture operation: " + op)
	}
}

func boundedSocket(kind int) (int, error) {
	fd, err := unix.Socket(unix.AF_UNIX, kind|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, option := range []int{unix.SO_RCVTIMEO, unix.SO_SNDTIMEO} {
		if err = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, option, &unix.Timeval{Sec: 1}); err != nil {
			_ = unix.Close(fd)
			return -1, err
		}
	}
	return fd, nil
}

func message(fd, kind int) (string, unix.Sockaddr, error) {
	var result []byte
	for len(result) < 4096 {
		b := make([]byte, 4096)
		n, from, err := unix.Recvfrom(fd, b, 0)
		if err != nil {
			return "", from, err
		}
		if n == 0 {
			return "", from, errors.New("empty fixture message")
		}
		result = append(result, b[:n]...)
		if kind != unix.SOCK_STREAM || strings.HasSuffix(string(result), "\n") {
			return strings.TrimSuffix(string(result), "\n"), from, nil
		}
	}
	return "", nil, errors.New("oversized fixture message")
}

// This identical syscall implementation serves the worker and host controls.
// Errno describes only the target connect/addressed-send operation. Subsequent
// echo failures cannot erase an unexpectedly successful operation.
func operate(t target, path, nonce string) attempt {
	a := attempt{Target: t.Name, Operation: t.Op, Path: path, Nonce: nonce}
	fd, err := boundedSocket(socketType(t.Op))
	if err != nil {
		a.SetupError = err.Error()
		return a
	}
	defer unix.Close(fd)
	if socketType(t.Op) == unix.SOCK_DGRAM {
		dir, e := os.MkdirTemp("/tmp", "slice-19-client-")
		if e != nil {
			a.SetupError = e.Error()
			return a
		}
		defer os.RemoveAll(dir)
		if e = unix.Bind(fd, &unix.SockaddrUnix{Name: filepath.Join(dir, "s")}); e != nil {
			a.SetupError = e.Error()
			return a
		}
	}
	to, payload := &unix.SockaddrUnix{Name: path}, []byte(nonce+"\n")
	a.Invoked = true
	switch t.Op {
	case "dgram-sendto":
		err = unix.Sendto(fd, payload, 0, to)
		a.Sent = err == nil
	case "dgram-sendmsg":
		var n int
		n, err = unix.SendmsgN(fd, payload, nil, to, 0)
		a.Sent = err == nil
		if err == nil && n != len(payload) {
			a.ExchangeError = "partial addressed send"
		}
	default:
		err = unix.Connect(fd, to)
		a.Connected = err == nil
	}
	if err != nil {
		a.Error = err.Error()
		var errno unix.Errno
		if errors.As(err, &errno) {
			a.Errno = int(errno)
		}
		return a
	}
	if a.Connected {
		n, e := unix.Write(fd, payload)
		a.Sent = e == nil && n == len(payload)
		if !a.Sent {
			a.ExchangeError = fmt.Sprintf("nonce write %d: %v", n, e)
			return a
		}
	}
	reply, _, e := message(fd, socketType(t.Op))
	a.Reply = reply
	if e != nil {
		a.ExchangeError = e.Error()
	}
	return a
}

// The server records a nonce BEFORE echoing it. A control's successful echo
// therefore proves receipt, including for unconnected datagram sends.
func listen(t target, path string, received func(string)) (func(), error) {
	fd, err := boundedSocket(socketType(t.Op))
	if err != nil {
		return nil, err
	}
	if err = unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err == nil {
		err = os.Chmod(path, 0600)
	}
	if err == nil && socketType(t.Op) != unix.SOCK_DGRAM {
		err = unix.Listen(fd, 16)
	}
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	// A stop flag prevents descriptor reuse after close. The one-second socket
	// deadline bounds the receiver's exit without sleep-based coordination.
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			peer := fd
			if socketType(t.Op) != unix.SOCK_DGRAM {
				peer, _, err = unix.Accept4(fd, unix.SOCK_CLOEXEC)
				if err != nil {
					continue
				}
				for _, option := range []int{unix.SO_RCVTIMEO, unix.SO_SNDTIMEO} {
					_ = unix.SetsockoptTimeval(peer, unix.SOL_SOCKET, option, &unix.Timeval{Sec: 1})
				}
			}
			value, from, e := message(peer, socketType(t.Op))
			if e == nil {
				received(value)
				if socketType(t.Op) == unix.SOCK_DGRAM {
					_ = unix.Sendto(peer, []byte(value+"\n"), 0, from)
				} else {
					_, _ = unix.Write(peer, []byte(value+"\n"))
				}
			}
			if peer != fd {
				_ = unix.Close(peer)
			}
		}
	}()
	return func() { close(stop); <-done; _ = unix.Close(fd) }, nil
}
