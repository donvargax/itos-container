// Package tcp is the egress proxy's adapter: it listens, reads each CONNECT head,
// dials a listed name, peeks the ClientHello and splices the tunnel. It holds no
// certificate and never completes a handshake, so it cannot intercept TLS. The
// domain decides every verdict; this package only carries them out.
package tcp

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/domain"
	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// settings holds every value a run uses. New fills all of them with the real
// ones, and no exported name can change them: only this package's tests build
// their own.
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

// New returns the proxy that writes its refusal log to log. It listens on every
// interface at 3128, since it is told no address of its own and Docker's
// inter-bridge isolation keeps other networks from its port.
func New(log port.Log) *Proxy {
	return newWith(log, settings{
		address:   "0.0.0.0:3128",
		headWait:  10 * time.Second,
		helloWait: 10 * time.Second,
		dialWait:  10 * time.Second,
		resolver:  net.DefaultResolver,
		listen:    listenTCP4,
		dial:      dialTCP4,
		limit:     domain.LogLimit,
	})
}

func newWith(log port.Log, s settings) *Proxy { return &Proxy{log: log, s: s} }

// listenTCP4 listens on an IPv4 address.
func listenTCP4(address string) (*net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp4", address)
	if err != nil {
		return nil, err
	}
	return net.ListenTCP("tcp4", addr)
}

// run is the state of one Serve: its connections, its log and its first failure.
type run struct {
	proxy    *Proxy
	list     domain.Allowlist
	listener *net.TCPListener
	log      *recorder
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	closing bool
	failure port.Failure
}

// Serve validates names, listens, writes the ready record and then accepts one
// goroutine per connection until ctx ends (nil), accepting fails, a record cannot
// be written or the log is full. Before it returns, it stops the log, closes the
// listener and every socket it holds and waits for its goroutines, so no record
// follows its return. It recovers no panic: one in any goroutine ends the process.
func (p *Proxy) Serve(parent context.Context, names []string) error {
	list, ok := domain.NewAllowlist(names)
	if !ok {
		return port.ListInvalid
	}
	listener, err := p.s.listen(p.s.address)
	if err != nil {
		return port.ListenFailed
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	r := &run{proxy: p, list: list, listener: listener, ctx: ctx, cancel: cancel, conns: map[net.Conn]struct{}{}}
	r.log = &recorder{out: p.log, numbers: domain.NewLog(p.s.limit), end: r.fail}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		<-ctx.Done()
		r.closeAll()
	}()
	if r.log.write(port.Record{Event: port.Ready}) {
		r.accept()
	}
	r.log.stop()
	cancel()
	<-closed
	r.wg.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure == "" {
		return nil
	}
	return r.failure
}

// accept serves connections until the listener fails or is closed.
func (r *run) accept() {
	for {
		client, err := r.listener.AcceptTCP()
		if err != nil {
			if r.ctx.Err() == nil {
				r.fail(port.AcceptFailed)
			}
			return
		}
		r.track(client)
		r.wg.Add(1)
		go r.handle(client)
	}
}

// fail records the first failure and ends the run.
func (r *run) fail(failure port.Failure) {
	r.mu.Lock()
	if r.failure == "" {
		r.failure = failure
	}
	r.mu.Unlock()
	r.cancel()
}

// track holds a socket so the end of the run can close it. A socket tracked after
// the run began closing is closed at once.
func (r *run) track(c net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[c] = struct{}{}
	if r.closing {
		_ = c.Close()
	}
}

// release closes a socket and forgets it.
func (r *run) release(c net.Conn) {
	r.mu.Lock()
	delete(r.conns, c)
	r.mu.Unlock()
	_ = c.Close()
}

// closeAll closes the listener and every socket held.
func (r *run) closeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closing = true
	_ = r.listener.Close()
	for c := range r.conns {
		_ = c.Close()
	}
}
