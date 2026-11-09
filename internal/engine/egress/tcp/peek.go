package tcp

import (
	"crypto/tls"
	"errors"
	"net"
	"slices"

	"github.com/donvargax/itos-container/internal/engine/egress/domain"
	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

var (
	errPeeked    = errors.New("client hello peeked")
	errHelloSize = errors.New("client hello exceeds its limit")
)

// shim is the net.Conn crypto/tls reads the hello through. Its Read serves the
// bytes that came after the head, then the socket, and records every byte it
// serves, up to HelloLimit. Its Write discards: crypto/tls sends an alert when
// the callback aborts, and a client must get nothing the proxy forged. Embedding
// the client's connection gives it every other method.
type shim struct {
	net.Conn
	tail     []byte
	served   int
	recorded []byte
}

func newShim(client net.Conn, tail []byte) *shim { return &shim{Conn: client, tail: tail} }

// Read serves the tail first and then the socket, never past HelloLimit in all.
func (s *shim) Read(p []byte) (int, error) {
	room := domain.HelloLimit - len(s.recorded)
	if room <= 0 {
		return 0, errHelloSize
	}
	p = p[:min(len(p), room)]
	var n int
	var err error
	if s.served < len(s.tail) {
		n = copy(p, s.tail[s.served:])
		s.served += n
	} else {
		n, err = s.Conn.Read(p)
	}
	s.recorded = append(s.recorded, p[:n]...)
	return n, err
}

// Write discards what crypto/tls writes and reports it written.
func (*shim) Write(p []byte) (int, error) { return len(p), nil }

// unserved is the part of the tail crypto/tls never asked for. It is not in the
// recording and the splice reads only the socket, so a pass must forward it.
func (s *shim) unserved() []byte { return s.tail[s.served:] }

// peek runs crypto/tls's server over the shim only as far as the ClientHello. Its
// one config sets GetConfigForClient and nothing else; the callback captures the
// server name and the extensions and always returns an error, so crypto/tls stops
// once the hello has parsed, before it chooses a version. The proxy holds no
// certificate and never completes a handshake. If the callback never ran, the
// hello is not Seen.
func peek(s *shim) port.Hello {
	var hello port.Hello
	config := &tls.Config{GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
		hello = port.Hello{Seen: true, ServerName: info.ServerName, Extensions: slices.Clone(info.Extensions)}
		return nil, errPeeked
	}}
	_ = tls.Server(s, config).Handshake()
	return hello
}
