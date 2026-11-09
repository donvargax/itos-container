package tcp

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
	"github.com/donvargax/itos-container/internal/engine/egress/port/porttest"
)

// silent is the answer rcode of a resolver that never replies.
const silent = 0xffff

type dnsAnswer func(qtype uint16) (rcode uint16, ips []net.IP)

// fakeDNS answers on a UDP socket on 127.0.0.1, with a header, the query's
// question and fixed A or AAAA records encoded by hand, since the maintained
// library for it is a module go.mod does not hold and the standard library's copy
// is internal. The resolver returned is Go's own, pointed at the socket.
func fakeDNS(t *testing.T, answer dnsAnswer) *net.Resolver {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			_, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			end := 12
			for buf[end] != 0 {
				end += 1 + int(buf[end])
			}
			end += 5
			rcode, ips := answer(binary.BigEndian.Uint16(buf[end-4:]))
			if rcode == silent {
				continue
			}
			reply := append([]byte(nil), buf[:2]...)
			reply = append(reply, 0x81, 0x80|byte(rcode), 0, 1)
			reply = binary.BigEndian.AppendUint16(reply, uint16(len(ips)))
			reply = append(reply, 0, 0, 0, 0)
			reply = append(reply, buf[12:end]...)
			for _, ip := range ips {
				kind, data := uint16(1), ip.To4()
				if data == nil {
					kind, data = 28, ip.To16()
				}
				reply = append(reply, 0xc0, 0x0c)
				reply = binary.BigEndian.AppendUint16(reply, kind)
				reply = append(reply, 0, 1, 0, 0, 0, 60)
				reply = binary.BigEndian.AppendUint16(reply, uint16(len(data)))
				reply = append(reply, data...)
			}
			_, _ = pc.WriteTo(reply, from)
		}
	}()
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp4", pc.LocalAddr().String())
	}}
}

// a answers A queries with the addresses and AAAA queries with an empty answer.
func a(addresses ...string) dnsAnswer {
	return func(qtype uint16) (uint16, []net.IP) {
		var ips []net.IP
		for _, address := range addresses {
			if qtype == 1 {
				ips = append(ips, net.ParseIP(address))
			}
		}
		return 0, ips
	}
}

var errStop = errors.New("stopped before connect")

// hookOnly dials with the adapter's own Control hook judging each address first;
// for one it passes, the wrapper returns an error, on which net stops before
// connect(2), so no packet is sent. Go's real dialer, resolver and the real table
// all still run.
func hookOnly(ctx context.Context, d *net.Dialer, address string) (*net.TCPConn, error) {
	hook := d.Control
	d.Control = func(network, addr string, c syscall.RawConn) error {
		if err := hook(network, addr, c); err != nil {
			return err
		}
		return errStop
	}
	return dial4(ctx, d, address)
}

func classify(t *testing.T, resolver *net.Resolver, dial func(context.Context, *net.Dialer, string) (*net.TCPConn, error), wait time.Duration) (*net.TCPConn, port.Event, port.Reason) {
	t.Helper()
	s := loopback()
	s.resolver, s.dial, s.dialWait = resolver, dial, wait
	return newWith(porttest.NewLog(), s).connect(context.Background(), "listed.example")
}

func TestDialClassification(t *testing.T) {
	cases := []struct {
		name   string
		answer dnsAnswer
		wait   time.Duration
		event  port.Event
		reason port.Reason
	}{
		{"a refused address then a valid one", a("10.0.0.1", "11.22.33.44"), testWait, port.UpstreamFailed, port.Connect},
		{"a valid address then a refused one", a("11.22.33.44", "10.0.0.1"), testWait, port.UpstreamFailed, port.Connect},
		{"a valid address alone", a("11.22.33.44"), testWait, port.UpstreamFailed, port.Connect},
		{"none valid", a("10.0.0.1", "192.168.1.1"), testWait, port.Refused, port.Address},
		{"no such host", func(uint16) (uint16, []net.IP) { return 3, nil }, testWait, port.UpstreamFailed, port.DNS},
		{"an empty answer", a(), testWait, port.UpstreamFailed, port.DNS},
		{"only an AAAA record", func(qtype uint16) (uint16, []net.IP) {
			if qtype == 28 {
				return 0, []net.IP{net.ParseIP("2001:db8::1")}
			}
			return 0, nil
		}, testWait, port.UpstreamFailed, port.DNS},
		{"a resolver that never replies", func(uint16) (uint16, []net.IP) { return silent, nil }, 300 * time.Millisecond, port.UpstreamFailed, port.DNS},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn, event, reason := classify(t, fakeDNS(t, c.answer), hookOnly, c.wait)
			if conn != nil {
				t.Errorf("a dial that stopped before connect returned a connection")
			}
			if event != c.event || reason != c.reason {
				t.Errorf("classified %s %s; want %s %s", event, reason, c.event, c.reason)
			}
		})
	}
	t.Run("a refused connection with the hook removed", func(t *testing.T) {
		closed, err := listen4("127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := closed.Addr().String()
		_ = closed.Close()
		_, closedPort, _ := net.SplitHostPort(address)
		dial := func(ctx context.Context, d *net.Dialer, _ string) (*net.TCPConn, error) {
			d.Control = nil
			return dial4(ctx, d, "listed.example.:"+closedPort)
		}
		conn, event, reason := classify(t, fakeDNS(t, a("127.0.0.1")), dial, testWait)
		if conn != nil || event != port.UpstreamFailed || reason != port.Connect {
			t.Errorf("classified %v %s %s; want connect from step four", conn, event, reason)
		}
	})
	t.Run("a refused address then a valid one that connects", func(t *testing.T) {
		listener, err := listen4("127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = listener.Close() }()
		_, listenerPort, _ := net.SplitHostPort(listener.Addr().String())
		var mu sync.Mutex
		var seen []string
		dial := func(ctx context.Context, d *net.Dialer, _ string) (*net.TCPConn, error) {
			hook := d.Control
			d.Control = func(network, addr string, c syscall.RawConn) error {
				mu.Lock()
				seen = append(seen, addr)
				mu.Unlock()
				if strings.HasPrefix(addr, "127.0.0.2:") {
					return hook(network, addr, c)
				}
				// The real connection goes to the loopback listener; the hook is
				// shown an address the table passes, as the record requires.
				return hook(network, "11.22.33.44:443", c)
			}
			return dial4(ctx, d, "listed.example.:"+listenerPort)
		}
		conn, event, reason := classify(t, fakeDNS(t, a("127.0.0.2", "127.0.0.1")), dial, testWait)
		if conn == nil {
			t.Fatalf("classified %s %s; want a connection to the valid address", event, reason)
		}
		_ = conn.Close()
		mu.Lock()
		defer mu.Unlock()
		if len(seen) != 2 || !strings.HasPrefix(seen[0], "127.0.0.2:") || !strings.HasPrefix(seen[1], "127.0.0.1:") {
			t.Errorf("hook saw %q; want 127.0.0.2 refused, then 127.0.0.1", seen)
		}
	})
	t.Run("the rooted name", func(t *testing.T) {
		var got string
		dial := func(_ context.Context, _ *net.Dialer, address string) (*net.TCPConn, error) {
			got = address
			return nil, errors.New("not dialled")
		}
		_, event, reason := classify(t, nil, dial, testWait)
		if got != "listed.example.:443" || event != port.UpstreamFailed || reason != port.Connect {
			t.Errorf("dialled %q, classified %s %s; want the rooted name on 443, and connect", got, event, reason)
		}
	})
	for name, c := range map[string]struct {
		answer dnsAnswer
		status string
		event  port.Event
		reason port.Reason
	}{
		"address": {a("10.0.0.1"), want403, port.Refused, port.Address},
		"dns":     {func(uint16) (uint16, []net.IP) { return 3, nil }, want502, port.UpstreamFailed, port.DNS},
		"connect": {a("11.22.33.44"), want502, port.UpstreamFailed, port.Connect},
	} {
		t.Run("through Serve: "+name, func(t *testing.T) {
			h := start(t, withResolver(fakeDNS(t, c.answer)), withDial(hookOnly))
			conn := h.dialClient()
			_, _ = conn.Write(connectHead("listed.example"))
			answer, _ := readUntilClosed(t, conn)
			if string(answer) != c.status {
				t.Errorf("answer = %q; want %q", answer, c.status)
			}
			records := h.records(2)
			h.quiet(2)
			wantRecord(t, records[1], 2, c.event, c.reason, "CONNECT", "listed.example", "443")
		})
	}
}

// TestRefusedAddressTable holds the first and last address of every row of the
// table, and the address on each side of it. The rows are written again here as
// first and last addresses, so the table's prefixes are not their own oracle.
func TestRefusedAddressTable(t *testing.T) {
	rows := [][2]string{
		{"0.0.0.0", "0.255.255.255"}, {"10.0.0.0", "10.255.255.255"}, {"100.64.0.0", "100.127.255.255"},
		{"127.0.0.0", "127.255.255.255"}, {"169.254.0.0", "169.254.255.255"}, {"172.16.0.0", "172.31.255.255"},
		{"192.0.0.0", "192.0.0.255"}, {"192.0.2.0", "192.0.2.255"}, {"192.31.196.0", "192.31.196.255"},
		{"192.52.193.0", "192.52.193.255"}, {"192.88.99.0", "192.88.99.255"}, {"192.168.0.0", "192.168.255.255"},
		{"192.175.48.0", "192.175.48.255"}, {"198.18.0.0", "198.19.255.255"}, {"198.51.100.0", "198.51.100.255"},
		{"203.0.113.0", "203.0.113.255"}, {"224.0.0.0", "239.255.255.255"}, {"240.0.0.0", "255.255.255.255"},
	}
	number := func(address string) uint32 {
		ip := net.ParseIP(address).To4()
		return binary.BigEndian.Uint32(ip)
	}
	inRows := func(n uint32) bool {
		for _, row := range rows {
			if n >= number(row[0]) && n <= number(row[1]) {
				return true
			}
		}
		return false
	}
	text := func(n uint32) string { return net.IP(binary.BigEndian.AppendUint32(nil, n)).String() + ":443" }
	for _, row := range rows {
		first, last := number(row[0]), number(row[1])
		for _, n := range []uint32{first, last, first + (last-first)/2} {
			if !addressRefused(text(n)) {
				t.Errorf("%s inside %v is not refused", text(n), row)
			}
		}
		if first > 0 && addressRefused(text(first-1)) != inRows(first-1) {
			t.Errorf("%s before %v: refused = %v, in a row = %v", text(first-1), row, addressRefused(text(first-1)), inRows(first-1))
		}
		if last < 0xffffffff && addressRefused(text(last+1)) != inRows(last+1) {
			t.Errorf("%s after %v: refused = %v, in a row = %v", text(last+1), row, addressRefused(text(last+1)), inRows(last+1))
		}
	}
	for _, passed := range []string{"11.22.33.44:443", "8.8.8.8:443", "1.1.1.1:1", "100.63.255.255:443", "100.128.0.0:443"} {
		if addressRefused(passed) {
			t.Errorf("%s is refused; want it to pass the table", passed)
		}
	}
	for _, text := range []string{"", "garbage", "11.22.33.44", "11.22.33.44:", "11.22.33.44:99999", "[::1]:443", "[2606:4700::1111]:443",
		"[::ffff:11.22.33.44]:443", "11.22.33.44:443 ", " 11.22.33.44:443", "example.com:443", "011.22.33.44:443"} {
		if !addressRefused(text) {
			t.Errorf("%q is not refused; want text that is not an IPv4 address and port refused", text)
		}
	}
}
