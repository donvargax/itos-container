// Package tcp is the egress proxy's adapter. This stub keeps what it is given,
// listens through its settings, closes each connection it accepts at once and
// returns nil when ctx ends; every other function returns zero values.
package tcp

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

type settings struct {
	address   string
	headWait  time.Duration
	helloWait time.Duration
	dialWait  time.Duration
	resolver  *net.Resolver
	listen    func(address string) (*net.TCPListener, error)
	dial      func(ctx context.Context, d *net.Dialer, address string) (*net.TCPConn, error)
	limit     int
}

// Proxy is the CONNECT proxy. It implements port.Proxy.
type Proxy struct {
	log port.Log
	s   settings
}

var _ port.Proxy = (*Proxy)(nil)

// New keeps only the log.
func New(log port.Log) *Proxy { return &Proxy{log: log} }

func newWith(log port.Log, s settings) *Proxy { return &Proxy{log: log, s: s} }

func listenTCP4(string) (*net.TCPListener, error) { return nil, nil }

func dialTCP4(context.Context, *net.Dialer, string) (*net.TCPConn, error) { return nil, nil }

// Serve listens, closes every connection at once and returns nil when ctx ends.
func (p *Proxy) Serve(ctx context.Context, _ []string) error {
	listener, err := p.s.listen(p.s.address)
	if err != nil {
		return port.ListenFailed
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			return nil
		}
		_ = conn.Close()
	}
}

type run struct {
	conns   map[net.Conn]struct{}
	closing bool
}

func (*run) track(net.Conn)   {}
func (*run) release(net.Conn) {}

var errHelloSize = errors.New("client hello exceeds its limit")

type shim struct {
	net.Conn
	tail     []byte
	recorded []byte
}

func newShim(client net.Conn, tail []byte) *shim { return &shim{Conn: client, tail: tail} }

func (*shim) Read([]byte) (int, error)  { return 0, nil }
func (*shim) Write([]byte) (int, error) { return 0, nil }
func (*shim) unserved() []byte          { return nil }
func peek(*shim) port.Hello             { return port.Hello{} }
func addressRefused(string) bool        { return false }

func (*Proxy) connect(context.Context, string) (*net.TCPConn, port.Event, port.Reason) {
	return nil, "", ""
}
