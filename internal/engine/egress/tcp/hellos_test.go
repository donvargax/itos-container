package tcp

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// scripted is a client socket that serves a script, then reports EOF, and counts
// anything written to it.
type scripted struct {
	net.Conn
	data    []byte
	written atomic.Int64
}

func (s *scripted) Read(p []byte) (int, error) {
	if len(s.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.data)
	s.data = s.data[n:]
	return n, nil
}

func (s *scripted) Write(p []byte) (int, error) {
	s.written.Add(int64(len(p)))
	return len(p), nil
}

// readEstablished reads the 200 a pass or a post-dial refusal begins with.
func readEstablished(t *testing.T, conn net.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(testWait))
	got := make([]byte, len(want200))
	if _, err := io.ReadFull(conn, got); err != nil || string(got) != want200 {
		t.Fatalf("answer = %q, %v; want the 200", got, err)
	}
}

// oneByteRecords frames payload as handshake records of one byte each.
func oneByteRecords(payload []byte) []byte {
	var out []byte
	for _, b := range payload {
		out = append(out, 0x16, 0x03, 0x03, 0x00, 0x01, b)
	}
	return out
}

func TestRefusedHellos(t *testing.T) {
	good := helloFor(t, "listed.example")
	claim := []byte{0x01, 0x00, 0xfd, 0xe8} // a ClientHello claiming 65,000 bytes
	long := append([]byte{0x16, 0x03, 0x03, 0x00, 0x04}, claim...)
	long = append(long, oneByteRecords(bytes.Repeat([]byte{'x'}, 70000/6))...)
	cases := []struct {
		name    string
		send    []byte
		closeIt bool
		trickle bool
		reason  port.Reason
	}{
		{"wrong SNI", helloFor(t, "other.example"), false, false, port.TLSName},
		{"upper-case SNI", bytes.Replace(good, []byte("listed.example"), []byte("LISTED.example"), 1), false, false, port.TLSName},
		{"no SNI", helloFor(t, ""), false, false, port.TLSNoName},
		{"ECH from Go's client", echHello(t, "public.example"), false, false, port.TLSECH},
		{"an empty 0xfe0d", withExtension(t, good, 0xfe0d, nil), false, false, port.TLSECH},
		{"a malformed 0xfe0d", withExtension(t, good, 0xfe0d, []byte{9, 9}), false, false, port.NotTLS},
		{"plain HTTP", []byte("GET / HTTP/1.1\r\nHost: listed.example\r\n\r\n"), false, false, port.NotTLS},
		{"an SSH banner", []byte("SSH-2.0-OpenSSH_9.9\r\n"), false, false, port.NotTLS},
		{"silence to the deadline", nil, false, false, port.NotTLS},
		{"truncation then silence", good[:100], false, false, port.NotTLS},
		{"truncation then close", good[:100], true, false, port.NotTLS},
		{"close with no bytes", nil, true, false, port.NotTLS},
		{"a record that starts like TLS and fails to parse", []byte{0x16, 0x03, 0x01, 0x00, 0x04, 0x01, 0x00, 0x00, 0x00}, false, false, port.NotTLS},
		{"70 KB of one-byte records", long, false, false, port.NotTLS},
		{"a hello trickled one byte every quarter deadline for four deadlines", good, false, true, port.NotTLS},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			upstream := listenSink(t, "")
			deadline := 400 * time.Millisecond
			h := start(t, toSink(upstream), within(testWait, deadline))
			conn := h.dialClient()
			_, _ = conn.Write(connectHead("listed.example"))
			readEstablished(t, conn)
			cut := false
			switch {
			case c.trickle:
				for i := 0; i < 16; i++ {
					if _, err := conn.Write(c.send[i : i+1]); err != nil {
						cut = true
					}
					time.Sleep(deadline / 4)
				}
			default:
				_, _ = conn.Write(c.send)
			}
			if c.closeIt {
				_ = conn.CloseWrite()
			}
			extra, _ := readUntilClosed(t, conn)
			if len(extra) != 0 {
				t.Errorf("client got %q after the 200; want no byte", extra)
			}
			upstream.closedByPeer(t)
			if got := upstream.received(); len(got) != 0 {
				t.Errorf("upstream got %d bytes; want none", len(got))
			}
			records := h.records(2)
			h.quiet(2)
			wantRecord(t, records[1], 2, port.Refused, c.reason, "CONNECT", "listed.example", "443")
			for _, text := range []string{"other.example", "public.example", "LISTED"} {
				if bytes.Contains(h.mem.Lines()[1], []byte(text)) {
					t.Errorf("record %q holds SNI text %q", h.mem.Lines()[1], text)
				}
			}
			if c.trickle && !cut {
				t.Error("the peek's one deadline did not cut the trickle off")
			}
		})
	}
}

// TestOlderECHCodePointsPass pins a disclosed limit: only 0xfe0d is refused as
// ECH. Hellos with the older code points and the exact SNI pass, whole.
func TestOlderECHCodePointsPass(t *testing.T) {
	for _, id := range []uint16{0xfe0a, 0xffce} {
		upstream := listenSink(t, "")
		h := start(t, toSink(upstream))
		hello := withExtension(t, helloFor(t, "listed.example"), id, []byte{1, 2, 3})
		conn := h.dialClient()
		_, _ = conn.Write(append(connectHead("listed.example"), hello...))
		readEstablished(t, conn)
		if got := upstream.waitFor(t, len(hello)); !bytes.Equal(got, hello) {
			t.Errorf("extension %#x: upstream got %d bytes, want the %d-byte hello whole", id, len(got), len(hello))
		}
		h.quiet(1)
	}
}

// TestShimLimit feeds a handshake message claiming 65,000 bytes in one-byte
// records: the recording stops at exactly 65,536 bytes and the next Read is refused.
func TestShimLimit(t *testing.T) {
	input := append([]byte{0x16, 0x03, 0x03, 0x00, 0x04, 0x01, 0x00, 0xfd, 0xe8}, oneByteRecords(bytes.Repeat([]byte{'x'}, 70000))...)
	client := &scripted{data: input[3000:]}
	sh := newShim(client, input[:3000])
	if hello := peek(sh); hello.Seen {
		t.Errorf("a hello that never parsed was Seen: %+v", hello)
	}
	if len(sh.recorded) != 65536 || !bytes.Equal(sh.recorded, input[:65536]) {
		t.Errorf("recorded %d bytes; want the first 65536 of the input", len(sh.recorded))
	}
	if n, err := sh.Read(make([]byte, 10)); n != 0 || !errors.Is(err, errHelloSize) {
		t.Errorf("Read past the limit = %d, %v; want 0 and the limit error", n, err)
	}
	if client.written.Load() != 0 {
		t.Errorf("the shim wrote %d bytes to the client", client.written.Load())
	}
}
