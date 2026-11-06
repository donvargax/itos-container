package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

// This fixture handles only one uncompressed IN A/AAAA question. It is not a
// production resolver and never forwards. Nonce names and fixed synthetic
// answers make cache hits and an unrelated DNS service observable.
func dnsQuery(name string, kind uint16) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b, 0x1414)
	binary.BigEndian.PutUint16(b[2:], 0x100)
	binary.BigEndian.PutUint16(b[4:], 1)
	for _, s := range strings.Split(name, ".") {
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	b = append(b, 0, byte(kind>>8), byte(kind), 0, 1)
	return b
}
func question(b []byte) (string, int, uint16) {
	if len(b) < 17 {
		return "", 0, 0
	}
	var labels []string
	i := 12
	for {
		if i >= len(b) {
			return "", 0, 0
		}
		n := int(b[i])
		i++
		if n == 0 {
			break
		}
		if n > 63 || i+n > len(b) {
			return "", 0, 0
		}
		labels = append(labels, string(b[i:i+n]))
		i += n
	}
	if i+4 > len(b) {
		return "", 0, 0
	}
	return strings.Join(labels, "."), i + 4, binary.BigEndian.Uint16(b[i:])
}
func dnsResponse(q []byte) []byte {
	name, end, kind := question(q)
	if end == 0 || !strings.HasSuffix(name, ".slice-14.test") {
		return nil
	}
	receipt(strings.TrimSuffix(name, ".slice-14.test"))
	b := append([]byte{}, q[:end]...)
	binary.BigEndian.PutUint16(b[2:], 0x8180)
	binary.BigEndian.PutUint16(b[6:], 1)
	binary.BigEndian.PutUint16(b[8:], 0)
	binary.BigEndian.PutUint16(b[10:], 0)
	ip := net.ParseIP("192.0.2.14").To4()
	if kind == 28 {
		ip = net.ParseIP("2001:db8::14").To16()
	} else if kind != 1 {
		return nil
	}
	b = append(b, 0xc0, 0x0c, byte(kind>>8), byte(kind), 0, 1, 0, 0, 0, 0, 0, byte(len(ip)))
	return append(b, ip...)
}
func dnsAnswer(b []byte) string {
	_, end, _ := question(b)
	if end == 0 || len(b) < end+12 || binary.BigEndian.Uint16(b[6:]) < 1 {
		return ""
	}
	n := int(binary.BigEndian.Uint16(b[end+10:]))
	if len(b) < end+12+n || (n != 4 && n != 16) {
		return ""
	}
	return net.IP(b[end+12 : end+12+n]).String()
}

func dnsAttempt(c config, r route, nonce string) (string, error) {
	if r.Protocol == "https" {
		return httpsDNS(c, r, nonce)
	}
	name := nonce + ".slice-14.test"
	if r.Protocol == "system4" || r.Protocol == "system6" {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		family := "ip4"
		if r.Protocol == "system6" {
			family = "ip6"
		}
		ips, e := net.DefaultResolver.LookupIP(ctx, family, name)
		if len(ips) > 0 {
			return ips[0].String(), e
		}
		return "", e
	}
	// Docker's embedded control uses a real network alias, with an address
	// independently recorded when the internal control network is prepared.
	if r.Protocol == "docker" {
		name = nonce
	}
	protocol := r.Protocol
	if protocol == "docker" {
		protocol = "udp4"
	}
	q := dnsQuery(name, 1)
	if strings.HasSuffix(r.Name, "aaaa") {
		q = dnsQuery(name, 28)
	}
	conn, e := net.DialTimeout(protocol, r.Address, 350*time.Millisecond)
	if e != nil {
		return "", e
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(350 * time.Millisecond))
	if strings.HasPrefix(protocol, "tcp") {
		q = append([]byte{byte(len(q) >> 8), byte(len(q))}, q...)
	}
	if _, e = conn.Write(q); e != nil {
		return "", e
	}
	b := make([]byte, 4096)
	if strings.HasPrefix(protocol, "tcp") {
		if _, e = io.ReadFull(conn, b[:2]); e != nil {
			return "", e
		}
		n := int(binary.BigEndian.Uint16(b))
		if n > len(b) {
			return "", fmt.Errorf("oversize DNS")
		}
		_, e = io.ReadFull(conn, b[:n])
		return dnsAnswer(b[:n]), e
	}
	n, e := conn.Read(b)
	return dnsAnswer(b[:n]), e
}

func serveDNS(c config) {
	addresses := append([]string{}, c.Resolvers...)
	addresses = append(addresses, "127.0.0.2", "172.30.14.2", "fd14:14::2")
	seen := map[string]bool{}
	for _, ip := range addresses {
		if seen[ip] {
			continue
		}
		seen[ip] = true
		a := net.JoinHostPort(ip, "53")
		p, e := net.ListenPacket("udp", a)
		must(e)
		go func() {
			b := make([]byte, 4096)
			for {
				n, addr, e := p.ReadFrom(b)
				if e != nil {
					return
				}
				if out := dnsResponse(b[:n]); out != nil {
					_, _ = p.WriteTo(out, addr)
				}
			}
		}()
		l, e := net.Listen("tcp", a)
		must(e)
		go func() {
			for {
				conn, e := l.Accept()
				if e != nil {
					return
				}
				go func() {
					defer conn.Close()
					_ = conn.SetDeadline(time.Now().Add(time.Second))
					var head [2]byte
					if _, e := io.ReadFull(conn, head[:]); e != nil {
						return
					}
					n := int(binary.BigEndian.Uint16(head[:]))
					if n > 4096 {
						return
					}
					b := make([]byte, n)
					if _, e := io.ReadFull(conn, b); e != nil {
						return
					}
					out := dnsResponse(b)
					if out == nil {
						return
					}
					_, _ = conn.Write(append([]byte{byte(len(out) >> 8), byte(len(out))}, out...))
				}()
			}
		}()
	}
	cert, e := tls.X509KeyPair([]byte(c.Cert), []byte(c.Key))
	must(e)
	l, e := tls.Listen("tcp", "172.30.14.2:8443", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	must(e)
	s := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, e := io.ReadAll(io.LimitReader(r.Body, 4096))
		if e != nil {
			http.Error(w, "bad fixture request", 400)
			return
		}
		out := dnsResponse(b)
		if out == nil {
			http.Error(w, "bad fixture question", 400)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(out)
	})}
	go func() { _ = s.Serve(l) }()
	// Observe the real embedded resolver queries on this control's loopback,
	// including Docker's port translation. Never impersonate 127.0.0.11.
	fd, e := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_DGRAM, 0x0300)
	must(e)
	go func() {
		b := make([]byte, 65536)
		for {
			n, _, e := syscall.Recvfrom(fd, b, 0)
			if e != nil {
				return
			}
			if n < 28 || b[0]>>4 != 4 || b[9] != 17 {
				continue
			}
			off := int(b[0]&15)*4 + 8
			if off+12 > n {
				continue
			}
			name, end, _ := question(b[off:n])
			if end != 0 && strings.Contains(name, "-docker-") {
				receipt(name)
			}
		}
	}()
}
