package tcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
	"github.com/donvargax/itos-container/internal/engine/egress/port/porttest"
)

// testWait bounds every wait that is not about a deadline, so a loaded runner
// cannot turn a slow read into another reason.
const testWait = 30 * time.Second

// harness runs one Serve against a loopback listener.
type harness struct {
	t      *testing.T
	log    port.Log
	mem    *porttest.Log
	addr   string
	cancel context.CancelFunc
	done   chan error
	dials  atomic.Int32
	once   sync.Once
	err    error
	waited atomic.Bool
}

type option func(*settings)

// start runs a proxy over a loopback listener with the list {listed.example}.
func start(t *testing.T, options ...option) *harness {
	t.Helper()
	mem := porttest.NewLog()
	return startOn(t, mem, mem, options...)
}

func startOn(t *testing.T, log port.Log, mem *porttest.Log, options ...option) *harness {
	t.Helper()
	h := &harness{t: t, log: log, mem: mem, done: make(chan error, 1)}
	listening := make(chan string, 1)
	s := settings{
		address: "127.0.0.1:0", headWait: testWait, helloWait: testWait, dialWait: testWait,
		resolver: net.DefaultResolver, dial: dial4, limit: 20000,
		listen: func(address string) (*net.TCPListener, error) {
			l, err := listen4(address)
			if err == nil {
				listening <- l.Addr().String()
			}
			return l, err
		},
	}
	for _, o := range options {
		o(&s)
	}
	inner := s.dial
	s.dial = func(ctx context.Context, d *net.Dialer, address string) (*net.TCPConn, error) {
		h.dials.Add(1)
		return inner(ctx, d, address)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() { h.done <- newWith(log, s).Serve(ctx, []string{"listed.example"}) }()
	select {
	case h.addr = <-listening:
	case err := <-h.done:
		t.Fatalf("Serve returned %v before listening", err)
	case <-time.After(testWait):
		t.Fatal("Serve did not listen")
	}
	t.Cleanup(func() {
		if err := h.stop(); err != nil && !h.waited.Load() {
			t.Errorf("Serve = %v after ctx ended; want nil", err)
		}
	})
	return h
}

// stop ends ctx and returns what Serve returned, waiting for it.
func (h *harness) stop() error {
	h.once.Do(func() {
		h.cancel()
		select {
		case h.err = <-h.done:
		case <-time.After(testWait):
			h.err = io.ErrNoProgress
			h.t.Error("Serve did not return after ctx ended")
		}
	})
	return h.err
}

// wait returns what Serve returns when it ends by itself.
func (h *harness) wait() error {
	h.t.Helper()
	h.waited.Store(true)
	select {
	case err := <-h.done:
		h.once.Do(func() { h.err = err })
		return err
	case <-time.After(testWait):
		h.t.Fatal("Serve did not return")
		return nil
	}
}

// dialClient opens a connection to the proxy.
func (h *harness) dialClient() *net.TCPConn {
	h.t.Helper()
	addr, err := net.ResolveTCPAddr("tcp4", h.addr)
	if err != nil {
		h.t.Fatal(err)
	}
	conn, err := net.DialTCP("tcp4", nil, addr)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// records waits until the log holds n lines, then returns them decoded, failing
// on a line that is not exactly one JSON object and a newline.
func (h *harness) records(n int) []port.Record {
	h.t.Helper()
	return waitRecords(h.t, h.mem, n)
}

func waitRecords(t *testing.T, log *porttest.Log, n int) []port.Record {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for len(log.Lines()) < n && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	lines := log.Lines()
	if len(lines) < n {
		t.Fatalf("log holds %d lines after %v; want at least %d: %q", len(lines), testWait, n, lines)
	}
	return decodeLines(t, lines)
}

func decodeLines(t *testing.T, lines [][]byte) []port.Record {
	t.Helper()
	records := make([]port.Record, 0, len(lines))
	for _, line := range lines {
		var record port.Record
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil || line[len(line)-1] != '\n' || len(line) > 2048 {
			t.Fatalf("log line %q is not one JSON object under 2 KiB and a newline: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

// quiet fails the test if the log has more than n lines after a short wait: a
// record is written before the client is answered, so this is for the negative.
func (h *harness) quiet(n int) {
	h.t.Helper()
	time.Sleep(50 * time.Millisecond)
	if lines := h.mem.Lines(); len(lines) != n {
		h.t.Fatalf("log holds %d lines, want %d: %q", len(lines), n, lines)
	}
}

// sink is a loopback upstream that keeps every byte it is sent.
type sink struct {
	ln     *net.TCPListener
	mu     sync.Mutex
	data   []byte
	closed bool
	conns  chan *net.TCPConn
}

// listenSink accepts connections and keeps what they send. A connection gets
// first before anything else, so a server that speaks first can be shown.
func listenSink(t *testing.T, first string) *sink {
	t.Helper()
	ln, err := listen4("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &sink{ln: ln, conns: make(chan *net.TCPConn, 16)}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.AcceptTCP()
			if err != nil {
				return
			}
			s.conns <- conn
			t.Cleanup(func() { _ = conn.Close() })
			if first != "" {
				_, _ = conn.Write([]byte(first))
			}
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := conn.Read(buf)
					s.mu.Lock()
					s.data = append(s.data, buf[:n]...)
					s.mu.Unlock()
					if err != nil {
						s.mu.Lock()
						s.closed = true
						s.mu.Unlock()
						return
					}
				}
			}()
		}
	}()
	return s
}

// received returns what upstream has been sent so far.
func (s *sink) received() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.data...)
}

// waitFor waits until the sink has n bytes and returns them.
func (s *sink) waitFor(t *testing.T, n int) []byte {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for len(s.received()) < n && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	return s.received()
}

// toSink makes the dial seam connect to the sink, whatever it is asked for.
func toSink(s *sink) option { return toAddr(s.ln.Addr().String()) }

// toAddr makes the dial seam connect to address, whatever it is asked for.
func toAddr(address string) option {
	return func(settings *settings) {
		settings.dial = func(ctx context.Context, _ *net.Dialer, _ string) (*net.TCPConn, error) {
			return dial4(ctx, &net.Dialer{}, address)
		}
	}
}

// listen4 and dial4 are the tests' own plumbing, so a test of the real ones fails
// at its assertions and no other test depends on them.
func listen4(address string) (*net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp4", address)
	if err != nil {
		return nil, err
	}
	return net.ListenTCP("tcp4", addr)
}

func dial4(ctx context.Context, d *net.Dialer, address string) (*net.TCPConn, error) {
	conn, err := d.DialContext(ctx, "tcp4", address)
	if err != nil {
		return nil, err
	}
	return conn.(*net.TCPConn), nil
}

// withDial replaces the dial seam.
func withDial(dial func(ctx context.Context, d *net.Dialer, address string) (*net.TCPConn, error)) option {
	return func(s *settings) { s.dial = dial }
}

// withResolver replaces the resolver the dialer is given.
func withResolver(r *net.Resolver) option { return func(s *settings) { s.resolver = r } }

// withLimit sets the refusal log's limit.
func withLimit(n int) option { return func(s *settings) { s.limit = n } }

// closedByPeer waits until the sink saw a connection end.
func (s *sink) closedByPeer(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("upstream connection was not closed")
}

// within sets the three waits a deadline test shortens.
func within(head, hello time.Duration) option {
	return func(s *settings) { s.headWait, s.helloWait = head, hello }
}

// readUntilClosed reads what the connection delivers until it ends, returning
// the bytes and whether it ended by EOF rather than a reset or a timeout.
func readUntilClosed(t *testing.T, conn net.Conn) ([]byte, bool) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(testWait))
	data, err := io.ReadAll(conn)
	return data, err == nil
}

// connectHead is the request line and headers of a CONNECT for name:443.
func connectHead(name string) []byte {
	return []byte("CONNECT " + name + ":443 HTTP/1.1\r\nHost: " + name + ":443\r\n\r\n")
}
