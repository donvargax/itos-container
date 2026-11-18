package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"syscall"
	"time"
)

// Invoked by the harness in ONLY the disposable proxy's network namespace.
// The worker never sees this mode and the host resolver is never changed.
func dnsControl() int {
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp4", "127.0.0.11:53")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := map[string]bool{}
	for _, name := range []string{"api.example.com", "dns.example", "connect.example"} {
		ips, err := resolver.LookupIP(ctx, "ip4", name+".")
		if name == "dns.example" {
			var dns *net.DNSError
			got[name] = errors.As(err, &dns) && dns.IsNotFound
			continue
		}
		got[name] = err == nil && len(ips) == 1
		if name == "connect.example" && got[name] {
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp4", net.JoinHostPort(ips[0].String(), "443"))
			if conn != nil {
				conn.Close()
			}
			got[name] = errors.Is(err, syscall.ECONNREFUSED)
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(got)
	for _, ok := range got {
		if !ok {
			return 1
		}
	}
	return 0
}
