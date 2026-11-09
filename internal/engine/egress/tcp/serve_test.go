package tcp

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
	"github.com/donvargax/itos-container/internal/engine/egress/port/porttest"
)

// failingAfter keeps the first n writes and fails every later one.
type failingAfter struct {
	mem   *porttest.Log
	after int32
	seen  atomic.Int32
}

func (l *failingAfter) Write(p []byte) (int, error) {
	if l.seen.Add(1) > l.after {
		return 0, errors.New("write failed")
	}
	return l.mem.Write(p)
}

// blocking keeps the first write at once and holds every later one until release.
type blocking struct {
	mem     *porttest.Log
	release chan struct{}
	seen    atomic.Int32
}

func (l *blocking) Write(p []byte) (int, error) {
	if l.seen.Add(1) > 1 {
		<-l.release
	}
	return l.mem.Write(p)
}

// refusedHead is the shortest refusal there is.
const refusedHead = "CONNECT other.example:443 HTTP/1.1\r\n\r\n"

// refuseMany sends refusedHead from n concurrent clients and waits for all to end.
func refuseMany(addr string, n int) {
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp4", addr)
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = conn.Write([]byte(refusedHead))
			_ = conn.SetReadDeadline(time.Now().Add(testWait))
			buf := make([]byte, 256)
			for {
				if _, err := conn.Read(buf); err != nil {
					return
				}
			}
		}()
	}
	wg.Wait()
}

// serveWithin returns what Serve returns for a run that must end by itself.
func serveWithin(t *testing.T, p *Proxy, names []string) error {
	t.Helper()
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { done <- p.Serve(ctx, names) }()
	select {
	case err := <-done:
		return err
	case <-time.After(testWait):
		t.Error("Serve did not return for a run that must end by itself")
		cancel()
		<-done
		return nil
	}
}

func TestServeFailures(t *testing.T) {
	names := []string{"listed.example"}
	t.Run("an invalid list never calls listen", func(t *testing.T) {
		s := loopback()
		s.listen = func(string) (*net.TCPListener, error) {
			t.Error("listen was called for an invalid list")
			return nil, errors.New("unused")
		}
		log := porttest.NewLog()
		if err := serveWithin(t, newWith(log, s), []string{"LISTED.example"}); err != port.ListInvalid {
			t.Errorf("Serve = %v; want %v", err, port.ListInvalid)
		}
		if len(log.Lines()) != 0 {
			t.Errorf("an invalid list wrote %d records", len(log.Lines()))
		}
	})
	t.Run("a failed listen", func(t *testing.T) {
		s := loopback()
		s.listen = func(string) (*net.TCPListener, error) { return nil, errors.New("address in use") }
		log := porttest.NewLog()
		if err := serveWithin(t, newWith(log, s), names); err != port.ListenFailed {
			t.Errorf("Serve = %v; want %v", err, port.ListenFailed)
		}
		if len(log.Lines()) != 0 {
			t.Errorf("a failed listen wrote %d records", len(log.Lines()))
		}
	})
	t.Run("a closed listener", func(t *testing.T) {
		s := loopback()
		s.listen = func(address string) (*net.TCPListener, error) {
			l, err := listen4(address)
			if err == nil {
				err = l.Close()
			}
			return l, err
		}
		log := porttest.NewLog()
		if err := serveWithin(t, newWith(log, s), names); err != port.AcceptFailed {
			t.Errorf("Serve = %v; want %v", err, port.AcceptFailed)
		}
		if records := decodeLines(t, log.Lines()); len(records) != 1 || records[0].Event != port.Ready {
			t.Errorf("a closed listener wrote %+v; want only ready", records)
		}
	})
	t.Run("a failing ready write", func(t *testing.T) {
		s := loopback()
		var listener *net.TCPListener
		s.listen = func(address string) (*net.TCPListener, error) {
			var err error
			listener, err = listen4(address)
			return listener, err
		}
		if err := serveWithin(t, newWith(porttest.FailingLog{}, s), names); err != port.LogFailed {
			t.Errorf("Serve = %v; want %v", err, port.LogFailed)
		}
		if listener == nil {
			t.Fatal("Serve never listened")
		}
		if conn, err := net.Dial("tcp4", listener.Addr().String()); err == nil {
			_ = conn.Close()
			t.Error("the listener was still open after Serve returned")
		}
	})
	t.Run("a failing write", func(t *testing.T) {
		mem := porttest.NewLog()
		h := startOn(t, &failingAfter{mem: mem, after: 1}, mem)
		conn := h.dialClient()
		_, _ = conn.Write([]byte(refusedHead))
		answer, _ := readUntilClosed(t, conn)
		if len(answer) != 0 {
			t.Errorf("a refusal whose record failed was answered %q; want no answer", answer)
		}
		if err := h.wait(); err != port.LogFailed {
			t.Errorf("Serve = %v; want %v", err, port.LogFailed)
		}
		if lines := mem.Lines(); len(lines) != 1 {
			t.Errorf("log holds %d lines after a failed write; want only ready", len(lines))
		}
	})
	t.Run("a limit of 2", func(t *testing.T) {
		h := start(t, withLimit(2))
		first := h.dialClient()
		_, _ = first.Write([]byte(refusedHead))
		if answer, _ := readUntilClosed(t, first); string(answer) != want403 {
			t.Errorf("first answer = %q; want the 403", answer)
		}
		second := h.dialClient()
		_, _ = second.Write([]byte(refusedHead))
		if answer, _ := readUntilClosed(t, second); len(answer) != 0 {
			t.Errorf("the refusal that filled the log was answered %q; want no answer", answer)
		}
		if err := h.wait(); err != port.LogExhausted {
			t.Errorf("Serve = %v; want %v", err, port.LogExhausted)
		}
		records := decodeLines(t, h.mem.Lines())
		events := []port.Event{port.Ready, port.Refused, port.Refused, port.LogFull}
		if len(records) != len(events) {
			t.Fatalf("log holds %d records; want ready, two refusals and log-full", len(records))
		}
		for i, event := range events {
			if records[i].Event != event || records[i].Seq != i+1 {
				t.Errorf("record %d = %+v; want %s with seq %d", i, records[i], event, i+1)
			}
		}
		wantRecord(t, records[3], 4, port.LogFull, "", "", "", "")
	})
	t.Run("a log-full record that cannot be written", func(t *testing.T) {
		mem := porttest.NewLog()
		h := startOn(t, &failingAfter{mem: mem, after: 2}, mem, withLimit(1))
		conn := h.dialClient()
		_, _ = conn.Write([]byte(refusedHead))
		_, _ = readUntilClosed(t, conn)
		if err := h.wait(); err != port.LogFailed {
			t.Errorf("Serve = %v; want %v", err, port.LogFailed)
		}
	})
}

// TestTrackAfterClosing holds that a socket tracked after the run began closing
// is closed at once, so a connection accepted as the run ends cannot outlive it.
func TestTrackAfterClosing(t *testing.T) {
	h := start(t)
	r := &run{conns: map[net.Conn]struct{}{}, closing: true}
	conn := h.dialClient()
	r.track(conn)
	if _, err := conn.Write([]byte("x")); !errors.Is(err, net.ErrClosed) {
		t.Errorf("Write on a socket tracked while closing = %v; want it closed", err)
	}
	r.release(conn)
	if len(r.conns) != 0 {
		t.Errorf("release left %d sockets tracked", len(r.conns))
	}
}

// TestLogFullUnderLoad has 50 concurrent refused clients meet a limit of 5: the log
// holds exactly ready, five records and log-full, whichever clients came first.
func TestLogFullUnderLoad(t *testing.T) {
	h := start(t, withLimit(5))
	refuseMany(h.addr, 50)
	if err := h.wait(); err != port.LogExhausted {
		t.Errorf("Serve = %v; want %v", err, port.LogExhausted)
	}
	records := decodeLines(t, h.mem.Lines())
	if len(records) != 7 {
		t.Fatalf("log holds %d records; want ready, five refusals and log-full", len(records))
	}
	for i, record := range records {
		event := port.Refused
		switch i {
		case 0:
			event = port.Ready
		case 6:
			event = port.LogFull
		}
		if record.Event != event || record.Seq != i+1 {
			t.Errorf("record %d = %+v; want %s with seq %d", i, record, event, i+1)
		}
	}
}

// TestRecordBeforeAnswer uses a log that blocks: no answer arrives until the write returns.
func TestRecordBeforeAnswer(t *testing.T) {
	mem := porttest.NewLog()
	log := &blocking{mem: mem, release: make(chan struct{})}
	h := startOn(t, log, mem)
	t.Cleanup(func() {
		select {
		case <-log.release:
		default:
			close(log.release)
		}
	})
	conn := h.dialClient()
	_, _ = conn.Write([]byte(refusedHead))
	_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	if n, _ := conn.Read(make([]byte, 64)); n != 0 {
		t.Fatalf("the client was answered with %d bytes while the record's write was blocked", n)
	}
	close(log.release)
	answer, _ := readUntilClosed(t, conn)
	if string(answer) != want403 {
		t.Errorf("answer = %q after the write returned; want the 403", answer)
	}
	if lines := mem.Lines(); len(lines) != 2 {
		t.Errorf("log holds %d lines; want ready and the refusal", len(lines))
	}
}

// TestConcurrentRecords has 50 concurrent refused clients leave seq 1 to 51 in
// the order the lines were written, one line each.
func TestConcurrentRecords(t *testing.T) {
	h := start(t)
	refuseMany(h.addr, 50)
	records := h.records(51)
	h.quiet(51)
	for i, record := range records {
		if record.Seq != i+1 {
			t.Fatalf("line %d has seq %d; want seq to be the order of the writes", i, record.Seq)
		}
		if i > 0 && (record.Event != port.Refused || record.Reason != port.NotListed) {
			t.Errorf("line %d = %+v; want a not-listed refusal", i, record)
		}
	}
}

// TestLongestRecord holds a 16-letter method, a 255-byte host and a 5-digit port
// within 2,048 bytes.
func TestLongestRecord(t *testing.T) {
	h := start(t)
	conn := h.dialClient()
	host := strings.Repeat("a", 255)
	_, _ = conn.Write([]byte("ABCDEFGHIJKLMNOP " + host + ":65535 HTTP/1.1\r\n\r\n"))
	_, _ = readUntilClosed(t, conn)
	records := h.records(2)
	wantRecord(t, records[1], 2, port.Refused, port.NotConnect, "ABCDEFGHIJKLMNOP", host, "65535")
	if n := len(h.mem.Lines()[1]); n > 2048 {
		t.Errorf("longest record is %d bytes; want at most 2048", n)
	}
}

func TestContextEndsTunnels(t *testing.T) {
	upstream := listenSink(t, "")
	h := start(t, toSink(upstream))
	hello := helloFor(t, "listed.example")
	conn := h.dialClient()
	_, _ = conn.Write(append(connectHead("listed.example"), hello...))
	readEstablished(t, conn)
	upstream.waitFor(t, len(hello))
	if err := h.stop(); err != nil {
		t.Errorf("Serve = %v after ctx ended; want nil", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(testWait))
	if n, err := conn.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Errorf("client read %d bytes, %v after ctx ended; want the tunnel closed", n, err)
	}
	upstream.closedByPeer(t)
}
