package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type route struct{ Name, Protocol, Address, Answer string }
type attempt struct {
	Name, Protocol, Address, Nonce, Result, Error string
	Attempted                                     bool
}

var outputMu sync.Mutex

func receipt(value string) {
	outputMu.Lock()
	defer outputMu.Unlock()
	emit(report{Kind: "receipt", Nonce: value})
}
func exchange(protocol, address, value string) (string, error) {
	c, e := net.DialTimeout(protocol, address, 350*time.Millisecond)
	if e != nil {
		return "", e
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(350 * time.Millisecond))
	if _, e = io.WriteString(c, value+"\n"); e != nil {
		return "", e
	}
	b := make([]byte, 1024)
	n, e := c.Read(b)
	return strings.TrimSpace(string(b[:n])), e
}
func attempts(c config, phase string) report {
	r := report{Kind: c.Mode, Nonce: c.Nonce, Phase: phase, Values: map[string]string{}}
	if c.Mode == "dns" {
		r.Values["resolv.conf"] = text("/etc/resolv.conf")
	}
	for _, p := range c.Routes {
		nonce := c.Nonce + "-" + phase + "-" + p.Name
		a := attempt{Name: p.Name, Protocol: p.Protocol, Address: p.Address, Nonce: nonce, Attempted: true}
		var e error
		if c.Mode == "dns" {
			a.Result, e = dnsAttempt(c, p, nonce)
		} else {
			a.Result, e = exchange(p.Protocol, p.Address, nonce)
		}
		if e != nil {
			a.Error = e.Error()
		}
		r.Attempts = append(r.Attempts, a)
	}
	return r
}

func listenEcho(protocol, address, identity string) {
	if strings.HasPrefix(protocol, "udp") {
		p, e := net.ListenPacket(protocol, address)
		must(e)
		go func() {
			defer p.Close()
			b := make([]byte, 2048)
			for {
				n, a, e := p.ReadFrom(b)
				if e != nil {
					return
				}
				value := strings.TrimSpace(string(b[:n]))
				receipt(value)
				_, _ = p.WriteTo([]byte(identity+":"+value), a)
			}
		}()
		return
	}
	l, e := net.Listen(protocol, address)
	must(e)
	go func() {
		defer l.Close()
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(time.Second))
				value, e := bufio.NewReader(c).ReadString('\n')
				if e == nil {
					value = strings.TrimSpace(value)
					receipt(value)
					_, _ = io.WriteString(c, identity+":"+value+"\n")
				}
			}()
		}
	}()
}

func ip(args ...string) {
	b, e := exec.Command("/bin/ip", args...).CombinedOutput()
	if e != nil {
		must(fmt.Errorf("control address operation failed: %w (%s)", e, b))
	}
}

func serve(c config) {
	// Control-only aliases on an internal bridge. No forwarding, host namespace
	// join, subject mutation or route to the public network is installed.
	assigned, err := net.InterfaceAddrs()
	must(err)
	seen := map[string]bool{}
	for _, address := range assigned {
		if p, ok := address.(*net.IPNet); ok {
			seen[p.IP.String()] = true
		}
	}
	for _, address := range c.Addresses {
		if seen[address] {
			continue
		}
		seen[address] = true
		prefix := "/32"
		if strings.Contains(address, ":") {
			prefix = "/128"
		}
		ip("addr", "add", address+prefix, "dev", "lo")
	}
	// IPv6 can exist on the runner while being disabled on a fresh container's
	// loopback, and newly assigned addresses can still be tentative (DAD).
	// Check both families in this isolated namespace before claiming readiness;
	// wait only for address availability, never drop a route or fake a denial.
	addresses := append([]string{"127.0.0.1", "::1", "172.30.14.2", "fd14:14::2"}, c.Addresses...)
	for _, address := range addresses {
		must(controlAddressReady(address))
	}
	if c.Mode == "dns" {
		// Reproduce the subject's independently captured system resolver targets
		// in this CONTROL only. The subject and daemon files are never changed.
		var lines []string
		for _, a := range c.Resolvers {
			lines = append(lines, "nameserver "+a)
		}
		lines = append(lines, "options attempts:1 timeout:1")
		must(os.WriteFile("/etc/resolv.conf", []byte(strings.Join(lines, "\n")+"\n"), 0600))
		serveDNS(c)
	} else {
		seen := map[string]bool{}
		for _, r := range c.Routes {
			if r.Answer != "control" {
				continue
			}
			key := r.Protocol + "/" + r.Address
			if seen[key] {
				continue
			}
			seen[key] = true
			listenEcho(r.Protocol, r.Address, "control")
		}
	}
	emit(report{Kind: "ready", Nonce: c.Nonce, Values: map[string]string{"ipv6-loopback": "ready"}})
	select {}
}

func controlAddressReady(address string) error {
	deadline := time.Now().Add(3 * time.Second)
	for {
		l, err := net.Listen("tcp", net.JoinHostPort(address, "0"))
		if err == nil {
			return l.Close()
		}
		if !errors.Is(err, syscall.EADDRNOTAVAIL) || !time.Now().Before(deadline) {
			return fmt.Errorf("control address %s unavailable (IPv6 all=%s lo=%s): %w", address, text("/proc/sys/net/ipv6/conf/all/disable_ipv6"), text("/proc/sys/net/ipv6/conf/lo/disable_ipv6"), err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func loopback(c config) {
	if c.Mode != "network" {
		return
	}
	a := net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Port))
	listenEcho("tcp4", a, "worker")
	v, e := exchange("tcp4", a, c.Nonce)
	emit(report{Kind: "loopback", Nonce: c.Nonce, Values: map[string]string{"reply": v, "error": fmt.Sprint(e)}})
}

func httpsDNS(c config, r route, nonce string) (string, error) {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(c.CA)) {
		return "", fmt.Errorf("missing fixture CA")
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "slice-14.test", MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	q := dnsQuery(nonce+".slice-14.test", 1)
	req, e := http.NewRequestWithContext(ctx, "POST", "https://"+r.Address+"/dns-query", strings.NewReader(string(q)))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "application/dns-message")
	resp, e := (&http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if e != nil {
		return "", e
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTPS DNS status %d", resp.StatusCode)
	}
	return dnsAnswer(b), nil
}
