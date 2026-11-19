package features

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/moby/term"
)

const fixtureTerminalDetachKeys = "ctrl-^,ctrl-_,ctrl-^,ctrl-_,ctrl-^,ctrl-_,ctrl-^,ctrl-_"

func TestOfflineRecorderTerminalAttachQuery(t *testing.T) {
	workerID := strings.Repeat("a", 64)
	pinned := "&detachKeys=" + url.QueryEscape(fixtureTerminalDetachKeys)
	for _, tc := range []struct {
		name          string
		terminal, tty bool
		query         string
		want          bool
	}{
		{"terminal attach", true, true, pinned, true},
		{"plain attach", false, false, "", true},
		{"nonterminal fixture", false, true, pinned, false},
		{"nonterminal worker", true, false, pinned, false},
		{"different keys", true, true, "&detachKeys=ctrl-p,ctrl-q", false},
		{"duplicate keys", true, true, pinned + pinned, false},
		{"unknown option", true, true, pinned + "&unknown=1", false},
		{"invalid stream flag", true, true, pinned + "&logs=invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, engine := createRecorderWorld(t, "plain")
			api := w.offline.api
			api.created, api.terminal = []string{workerID}, tc.terminal
			w.session.token = strings.Repeat("b", 64)
			engine.workers[workerID] = map[string]any{
				"Id": workerID, "Name": "/itos-container-" + w.session.token, "Image": w.doctor.imageID,
				"Config": map[string]any{"Labels": w.sessionLabels(), "Tty": tc.tty},
			}
			path := "/containers/" + workerID + "/attach"
			req := httptest.NewRequest(http.MethodPost, path+"?stream=1&stdin=1&stdout=1&stderr=1"+tc.query, nil)
			got := api.allowed(req, &offlineRequest{Method: req.Method, Path: path})
			if api.problem != nil {
				t.Fatal(api.problem)
			}
			if got != tc.want {
				t.Errorf("recorder allowed attach = %t, want %t", got, tc.want)
			}
		})
	}
}

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
	pinned, client := newMemConn(t, fixtureTerminalDetachKeys)
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
