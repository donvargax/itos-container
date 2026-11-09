package tcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// closedConn is a dial result whose socket is already closed, so a write fails.
func closedConn(t *testing.T, s *sink) func(context.Context, *net.Dialer, string) (*net.TCPConn, error) {
	t.Helper()
	return func(ctx context.Context, _ *net.Dialer, _ string) (*net.TCPConn, error) {
		conn, err := dial4(ctx, &net.Dialer{}, s.ln.Addr().String())
		if err == nil {
			err = conn.Close()
		}
		return conn, err
	}
}

// twoRecords splits a single-record hello into two records over its handshake message.
func twoRecords(hello []byte) []byte {
	payload := hello[5:]
	cut := len(payload) / 2
	var out []byte
	for _, part := range [][]byte{payload[:cut], payload[cut:]} {
		out = append(out, hello[0], hello[1], hello[2])
		out = binary.BigEndian.AppendUint16(out, uint16(len(part)))
		out = append(out, part...)
	}
	return out
}

func TestReplay(t *testing.T) {
	hello := helloFor(t, "listed.example")
	for _, trailer := range []int{0, 37, 600, 6000} {
		t.Run("a trailer of "+strconv.Itoa(trailer)+" bytes in one write", func(t *testing.T) {
			upstream := listenSink(t, "")
			h := start(t, toSink(upstream))
			tail := bytes.Repeat([]byte{0xa5}, trailer)
			conn := h.dialClient()
			_, _ = conn.Write(append(append(connectHead("listed.example"), hello...), tail...))
			readEstablished(t, conn)
			want := append(append([]byte(nil), hello...), tail...)
			if got := upstream.waitFor(t, len(want)); !bytes.Equal(got, want) {
				t.Errorf("upstream got %d bytes, want the hello and a %d-byte trailer exactly (%d)", len(got), trailer, len(want))
			}
			h.quiet(1)
		})
	}
	t.Run("a hello split byte by byte", func(t *testing.T) {
		upstream := listenSink(t, "")
		h := start(t, toSink(upstream))
		conn := h.dialClient()
		_, _ = conn.Write(connectHead("listed.example"))
		readEstablished(t, conn)
		for i := range hello {
			if _, err := conn.Write(hello[i : i+1]); err != nil {
				t.Fatal(err)
			}
		}
		if got := upstream.waitFor(t, len(hello)); !bytes.Equal(got, hello) {
			t.Errorf("upstream got %d bytes; want the %d-byte hello exactly", len(got), len(hello))
		}
	})
	t.Run("a hello over two TLS records", func(t *testing.T) {
		upstream := listenSink(t, "")
		h := start(t, toSink(upstream))
		split := twoRecords(hello)
		conn := h.dialClient()
		_, _ = conn.Write(append(connectHead("listed.example"), split...))
		readEstablished(t, conn)
		if got := upstream.waitFor(t, len(split)); !bytes.Equal(got, split) {
			t.Errorf("upstream got %d bytes; want the %d bytes of both records exactly", len(got), len(split))
		}
	})
	// The head arrives in two writes, cut inside its CRLF CRLF after CR LF CR,
	// under a head deadline longer than the test's own wait: a head read that took
	// one Read, or searched only its latest read for the terminator, would refuse
	// it or answer only at the deadline.
	t.Run("a head in two writes cut inside its terminator", func(t *testing.T) {
		upstream := listenSink(t, "")
		h := start(t, toSink(upstream), within(120*time.Second, testWait))
		conn := h.dialClient()
		_, _ = conn.Write([]byte("CONNECT listed.example:443 HTTP/1.1\r\n\r"))
		time.Sleep(100 * time.Millisecond)
		_, _ = conn.Write([]byte("\n"))
		readEstablished(t, conn)
		_, _ = conn.Write(hello)
		if got := upstream.waitFor(t, len(hello)); !bytes.Equal(got, hello) {
			t.Errorf("upstream got %d bytes; want the %d-byte hello exactly", len(got), len(hello))
		}
		h.quiet(1)
	})
	// With the hello sent with the head, the peek reads no socket, so only a
	// cleared deadline lets a later write cross after the hello deadline has passed.
	t.Run("the deadline is cleared on a pass", func(t *testing.T) {
		upstream := listenSink(t, "")
		h := start(t, toSink(upstream), within(testWait, 300*time.Millisecond))
		conn := h.dialClient()
		_, _ = conn.Write(append(connectHead("listed.example"), hello...))
		readEstablished(t, conn)
		time.Sleep(700 * time.Millisecond)
		_, _ = conn.Write([]byte("late"))
		want := append(append([]byte(nil), hello...), "late"...)
		if got := upstream.waitFor(t, len(want)); !bytes.Equal(got, want) {
			t.Errorf("upstream got %d bytes; want the hello and the late write (%d)", len(got), len(want))
		}
		h.quiet(1)
	})
	t.Run("a failed replay", func(t *testing.T) {
		upstream := listenSink(t, "")
		h := start(t, withDial(closedConn(t, upstream)))
		conn := h.dialClient()
		_, _ = conn.Write(append(connectHead("listed.example"), hello...))
		readEstablished(t, conn)
		extra, _ := readUntilClosed(t, conn)
		if len(extra) != 0 {
			t.Errorf("client got %q after the 200; want none", extra)
		}
		records := h.records(2)
		h.quiet(2)
		wantRecord(t, records[1], 2, port.UpstreamFailed, port.Connect, "CONNECT", "listed.example", "443")
	})
}

func TestSplice(t *testing.T) {
	hello := helloFor(t, "listed.example")
	established := func(t *testing.T, upstream *sink, h *harness) (*net.TCPConn, *net.TCPConn) {
		t.Helper()
		conn := h.dialClient()
		_, _ = conn.Write(append(connectHead("listed.example"), hello...))
		readEstablished(t, conn)
		server := <-upstream.conns
		upstream.waitFor(t, len(hello))
		return conn, server
	}
	t.Run("half-close from the client", func(t *testing.T) {
		upstream := listenSink(t, "")
		h := start(t, toSink(upstream))
		conn, server := established(t, upstream, h)
		_ = conn.CloseWrite()
		upstream.closedByPeer(t)
		if _, err := server.Write([]byte("bye")); err != nil {
			t.Fatal(err)
		}
		_ = server.CloseWrite()
		got, ok := readUntilClosed(t, conn)
		if string(got) != "bye" || !ok {
			t.Errorf("client read %q, EOF %v; want bye then EOF after the half-close", got, ok)
		}
	})
	t.Run("half-close from the server", func(t *testing.T) {
		upstream := listenSink(t, "")
		h := start(t, toSink(upstream))
		conn, server := established(t, upstream, h)
		_ = server.CloseWrite()
		_ = conn.SetReadDeadline(time.Now().Add(testWait))
		if n, err := conn.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
			t.Errorf("client read %d bytes, %v; want EOF", n, err)
		}
		_, _ = conn.Write([]byte("after"))
		if got := upstream.waitFor(t, len(hello)+5); !bytes.Equal(got[len(hello):], []byte("after")) {
			t.Errorf("upstream got %q after the server's half-close; want after", got[len(hello):])
		}
	})
	t.Run("a server that speaks first reaches the client only after the hello passes", func(t *testing.T) {
		upstream := listenSink(t, "SERVER-FIRST")
		h := start(t, toSink(upstream))
		conn := h.dialClient()
		_, _ = conn.Write(connectHead("listed.example"))
		readEstablished(t, conn)
		_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		if n, err := conn.Read(make([]byte, 64)); n != 0 {
			t.Fatalf("client read %d bytes, %v before the hello; the server's first words crossed", n, err)
		}
		_, _ = conn.Write(hello)
		_ = conn.SetReadDeadline(time.Now().Add(testWait))
		got := make([]byte, len("SERVER-FIRST"))
		if n, err := conn.Read(got); err != nil || string(got[:n]) != "SERVER-FIRST" {
			t.Errorf("client read %q, %v after the hello; want SERVER-FIRST", got[:n], err)
		}
	})
	t.Run("a reset on one side closes the other", func(t *testing.T) {
		upstream := listenSink(t, "")
		h := start(t, toSink(upstream))
		conn, server := established(t, upstream, h)
		_ = server.SetLinger(0)
		_ = server.Close()
		_ = conn.SetReadDeadline(time.Now().Add(testWait))
		if n, err := conn.Read(make([]byte, 1)); n != 0 || (!errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET)) {
			t.Errorf("client read %d bytes, %v; want EOF or a connection reset", n, err)
		}
	})
}
