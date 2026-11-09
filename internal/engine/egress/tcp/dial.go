package tcp

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"sync/atomic"
	"syscall"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

var errAddress = errors.New("address refused")

// special is the IANA IPv4 special-purpose registry (read 2026-11-09, last
// updated 2025-10-09) plus multicast, with the registry's narrower rows inside
// these. A listed name that resolves into it is refused.
var special = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"),
	netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

// addressRefused is the Control hook's rule for the address a dial is about to
// connect to, as text: anything that is not an IPv4 address and port, and any
// IPv4 address in the table.
func addressRefused(text string) bool {
	addr, err := netip.ParseAddrPort(text)
	if err != nil || !addr.Addr().Is4() {
		return true
	}
	return slices.ContainsFunc(special, func(prefix netip.Prefix) bool { return prefix.Contains(addr.Addr()) })
}

// dialTCP4 dials over IPv4 with the dialer it is given.
func dialTCP4(ctx context.Context, d *net.Dialer, address string) (*net.TCPConn, error) {
	conn, err := d.DialContext(ctx, "tcp4", address)
	if err != nil {
		return nil, err
	}
	return conn.(*net.TCPConn), nil
}

// connect dials a listed name on 443. The Control hook sees the address actually
// connected, CNAME targets and changed answers included, and refuses the table's.
// A failed dial is classified in four steps: an error holding a *net.DNSError is
// dns; the hook having passed an address is connect; having refused one and
// passed none is address; neither is connect. The returned error cannot decide
// between the last three, since the dialer keeps the first address's error. It
// returns the failure's event and reason, or the connection.
func (p *Proxy) connect(ctx context.Context, name string) (*net.TCPConn, port.Event, port.Reason) {
	var passed, refused atomic.Bool
	dialer := &net.Dialer{
		Timeout:  p.s.dialWait,
		Resolver: p.s.resolver,
		Control: func(_, address string, _ syscall.RawConn) error {
			if addressRefused(address) {
				refused.Store(true)
				return errAddress
			}
			passed.Store(true)
			return nil
		},
	}
	conn, err := p.s.dial(ctx, dialer, name+".:443")
	var dnsErr *net.DNSError
	switch {
	case err == nil:
		return conn, "", ""
	case errors.As(err, &dnsErr):
		return nil, port.UpstreamFailed, port.DNS
	case !passed.Load() && refused.Load():
		return nil, port.Refused, port.Address
	}
	return nil, port.UpstreamFailed, port.Connect
}
