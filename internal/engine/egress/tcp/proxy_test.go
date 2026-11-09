package tcp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
	"github.com/donvargax/itos-container/internal/engine/egress/port/porttest"
)

// loopback is the settings every test starts from: a loopback listener on a free
// port and long waits.
func loopback() settings {
	return settings{
		address: "127.0.0.1:0", headWait: testWait, helloWait: testWait, dialWait: testWait,
		resolver: net.DefaultResolver, listen: listen4, dial: dial4, limit: 20000,
	}
}

func TestAdapterContract(t *testing.T) {
	porttest.ProxyContract(t, func(f porttest.ProxyFixture) port.Proxy { return newWith(f.Log, loopback()) })
}

func TestNewBindsTheRealOnes(t *testing.T) {
	p := New(porttest.NewLog())
	if p.s.address != "0.0.0.0:3128" {
		t.Errorf("address = %q; want 0.0.0.0:3128", p.s.address)
	}
	for name, got := range map[string]time.Duration{"head": p.s.headWait, "hello": p.s.helloWait, "dial": p.s.dialWait} {
		if got != 10*time.Second {
			t.Errorf("%s wait = %v; want 10s", name, got)
		}
	}
	if p.s.resolver != net.DefaultResolver {
		t.Errorf("resolver = %v; want net.DefaultResolver", p.s.resolver)
	}
	if p.s.limit != 20000 {
		t.Errorf("limit = %d; want 20000", p.s.limit)
	}
	if got, want := reflect.ValueOf(p.s.listen).Pointer(), reflect.ValueOf(listenTCP4).Pointer(); got != want {
		t.Error("listen is not the real listen function")
	}
	if got, want := reflect.ValueOf(p.s.dial).Pointer(), reflect.ValueOf(dialTCP4).Pointer(); got != want {
		t.Error("dial is not the real dial function")
	}
	realListenAndDial(t)
}

// realListenAndDial holds the two real functions New fills in by what they do.
func realListenAndDial(t *testing.T) {
	listener, err := listenTCP4("127.0.0.1:0")
	if err != nil || listener == nil {
		t.Fatalf("listenTCP4 = %v, %v; want a listener", listener, err)
	}
	defer func() { _ = listener.Close() }()
	if network := listener.Addr().Network(); network != "tcp" || listener.Addr().(*net.TCPAddr).IP.To4() == nil {
		t.Errorf("listener is %v %v; want an IPv4 TCP listener", network, listener.Addr())
	}
	if _, err := listenTCP4("not an address"); err == nil {
		t.Error("listenTCP4 accepted a malformed address")
	}
	accepted := make(chan struct{})
	go func() {
		conn, err := listener.AcceptTCP()
		if err == nil {
			_ = conn.Close()
		}
		close(accepted)
	}()
	conn, err := dialTCP4(context.Background(), &net.Dialer{}, listener.Addr().String())
	if err != nil || conn == nil {
		t.Fatalf("dialTCP4 = %v, %v; want a connection", conn, err)
	}
	_ = conn.Close()
	<-accepted
	if _, err := dialTCP4(context.Background(), &net.Dialer{}, "not an address"); err == nil {
		t.Error("dialTCP4 accepted a malformed address")
	}
}

// TestVerifiedThroughTheTunnel has Go's client verify against a pool holding only
// the upstream's own certificate: the proxy holds no certificate of its own and
// never completes a handshake, so the client sees the upstream's.
func TestVerifiedThroughTheTunnel(t *testing.T) {
	cert := upstreamCertificate(t, "listed.example")
	upstream, err := tls.Listen("tcp4", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = upstream.Close() }()
	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.Write([]byte("hello from upstream")); err == nil {
			_ = conn.(*tls.Conn).CloseWrite()
		}
		_, _ = io.Copy(io.Discard, conn)
	}()
	h := start(t, toAddr(upstream.Addr().String()))
	conn := h.dialClient()
	_, _ = conn.Write(connectHead("listed.example"))
	readEstablished(t, conn)
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	client := tls.Client(conn, &tls.Config{ServerName: "listed.example", RootCAs: pool, MinVersion: tls.VersionTLS12})
	_ = client.SetDeadline(time.Now().Add(testWait))
	if err := client.Handshake(); err != nil {
		t.Fatalf("handshake through the tunnel: %v", err)
	}
	seen := client.ConnectionState().PeerCertificates
	if len(seen) != 1 || !seen[0].Equal(leaf) {
		t.Errorf("client saw %d certificates, first %v; want the upstream's own leaf", len(seen), seen)
	}
	data, err := io.ReadAll(client)
	if err != nil || string(data) != "hello from upstream" {
		t.Errorf("read %q, %v through the tunnel; want the upstream's bytes", data, err)
	}
	h.quiet(1)
}
