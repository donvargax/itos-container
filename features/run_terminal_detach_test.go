package features

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/moby/term"
)

// The recorder's TTY escape model must match the daemon's copyEscapable: an
// absent detachKeys keeps Docker's default ctrl-p,ctrl-q, which is swallowed
// and the attach dropped, while a supplied sequence is applied instead and
// every other byte passes through unchanged. This runs the same moby/term
// proxy the daemon runs, over an in-memory connection.
func TestDetachEscapeConnMatchesTheDaemon(t *testing.T) {
	defaults, client := newMemConn(t, "")
	if defaults == nil {
		t.Fatal("invalid default escape sequence")
	}
	go func() { _, _ = client.Write([]byte("\x10\x11A")) }()
	received := make([]byte, 3)
	if err := readDeadline(defaults, received); err == nil {
		t.Fatalf("the daemon default let bytes through: %q", received)
	} else {
		var escape term.EscapeError
		if !errors.As(err, &escape) {
			t.Fatalf("the daemon default did not report an escape: %v", err)
		}
	}
	pinned, client := newMemConn(t, "ctrl-^,ctrl-_,ctrl-^,ctrl-_,ctrl-^,ctrl-_,ctrl-^,ctrl-_")
	go func() { _, _ = client.Write([]byte("\x10\x11A")) }()
	received = make([]byte, 3)
	if err := readDeadline(pinned, received); err != nil {
		t.Fatal(err)
	}
	if string(received) != "\x10\x11A" {
		t.Fatalf("the pinned sequence filtered workload bytes: %q", received)
	}
	if _, client := newMemConn(t, "ctrl-not-a-key"); client != nil {
		t.Fatal("an invalid escape sequence was accepted")
	}
}

func readDeadline(conn net.Conn, into []byte) error {
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	_, err := io.ReadFull(conn, into)
	return err
}

// newMemConn returns the wrapped endpoint and its peer.
func newMemConn(t *testing.T, keys string) (net.Conn, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	wrapped, err := newDetachEscapeConn(server, keys)
	if err != nil {
		_ = server.Close()
		_ = client.Close()
		return nil, nil
	}
	return wrapped, client
}
