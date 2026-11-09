package tcp

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

const (
	want200 = "HTTP/1.1 200 Connection Established\r\n\r\n"
	want403 = "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	want502 = "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
)

// wantRecord holds one decoded record to what a refusal must write.
func wantRecord(t *testing.T, got port.Record, seq int, event port.Event, reason port.Reason, method, target, portText string) {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, got.Time)
	if err != nil || !strings.HasSuffix(got.Time, "Z") || parsed.UTC().Format(time.RFC3339) != got.Time {
		t.Errorf("record time %q is not RFC 3339 UTC to the second", got.Time)
	}
	want := port.Record{Schema: 1, Seq: seq, Time: got.Time, Event: event, Method: method, Target: target, Port: portText, Reason: reason}
	if got != want {
		t.Errorf("record = %+v; want %+v", got, want)
	}
}

func TestRefusedHeads(t *testing.T) {
	filler := "CONNECT listed.example:443 HTTP/1.1\r\nX: "
	cases := []struct {
		name    string
		input   string
		partial bool
		reason  port.Reason
		fields  [3]string
	}{
		{"not listed", "CONNECT other.example:443 HTTP/1.1\r\n\r\n", false, port.NotListed, [3]string{"CONNECT", "other.example", "443"}},
		{"wrong port", "CONNECT listed.example:80 HTTP/1.1\r\n\r\n", false, port.Port, [3]string{"CONNECT", "listed.example", "80"}},
		{"not connect with markers in the url and Proxy-Authorization",
			"GET http://listed.example/path?token=secret HTTP/1.1\r\nProxy-Authorization: Basic c2VjcmV0\r\n\r\n",
			false, port.NotConnect, [3]string{"GET", "", ""}},
		{"userinfo marker", "CONNECT secret-token@listed.example:443 HTTP/1.1\r\n\r\n", false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"path marker", "CONNECT listed.example:443/path?token=secret HTTP/1.1\r\n\r\n", false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"exactly 8192 bytes and no terminator", filler + strings.Repeat("a", 8192-len(filler)), false, port.Malformed, [3]string{}},
		{"bare LF", "CONNECT listed.example:443 HTTP/1.1\n\n", true, port.Malformed, [3]string{}},
		{"bare LF in the request line", "CONNECT listed.example:443 HTTP/1.1\nHost: x\r\n\r\n", false, port.Malformed, [3]string{}},
		{"partial head then close", "CONNECT listed.ex", true, port.Malformed, [3]string{}},
		{"one byte then close", "C", true, port.Malformed, [3]string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := start(t)
			conn := h.dialClient()
			if _, err := conn.Write([]byte(c.input)); err != nil {
				t.Fatal(err)
			}
			if c.partial {
				_ = conn.CloseWrite()
			}
			answer, _ := readUntilClosed(t, conn)
			if string(answer) != want403 {
				t.Fatalf("answer = %q; want exactly the 403", answer)
			}
			records := h.records(2)
			h.quiet(2)
			wantRecord(t, records[0], 1, port.Ready, "", "", "", "")
			wantRecord(t, records[1], 2, port.Refused, c.reason, c.fields[0], c.fields[1], c.fields[2])
			for _, marker := range []string{"secret", "c2VjcmV0", "token", "/path"} {
				if bytes.Contains(h.mem.Lines()[1], []byte(marker)) {
					t.Errorf("record %q holds the marker %q", h.mem.Lines()[1], marker)
				}
			}
			if h.dials.Load() != 0 {
				t.Errorf("a refused head called the dial seam %d times", h.dials.Load())
			}
		})
	}
}

func TestEmptyConnections(t *testing.T) {
	t.Run("close before the first byte", func(t *testing.T) {
		h := start(t)
		conn := h.dialClient()
		_ = conn.Close()
		h.quiet(1)
	})
	t.Run("reset before the first byte", func(t *testing.T) {
		h := start(t)
		conn := h.dialClient()
		_ = conn.SetLinger(0)
		_ = conn.Close()
		h.quiet(1)
	})
	deadline := 300 * time.Millisecond
	for name, send := range map[string]string{"silence to the deadline": "", "a partial head then silence": "CONNECT listed.ex"} {
		t.Run(name, func(t *testing.T) {
			h := start(t, within(deadline, testWait))
			conn := h.dialClient()
			_, _ = conn.Write([]byte(send))
			answer, _ := readUntilClosed(t, conn)
			if string(answer) != want403 {
				t.Errorf("answer = %q; want the 403", answer)
			}
			records := h.records(2)
			h.quiet(2)
			wantRecord(t, records[1], 2, port.Refused, port.Malformed, "", "", "")
		})
	}
	t.Run("a trickle of one byte every quarter deadline for four deadlines", func(t *testing.T) {
		h := start(t, within(deadline, testWait))
		conn := h.dialClient()
		var cut atomic.Bool
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			for i := 0; i < 16; i++ {
				if _, err := conn.Write([]byte{'a'}); err != nil {
					cut.Store(true)
				}
				time.Sleep(deadline / 4)
			}
		}()
		_, _ = readUntilClosed(t, conn)
		<-finished
		if !cut.Load() {
			t.Error("the deadline did not cut the trickle off: each byte extended it")
		}
		records := h.records(2)
		wantRecord(t, records[1], 2, port.Refused, port.Malformed, "", "", "")
	})
}
