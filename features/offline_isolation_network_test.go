package features

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

func (s *isolationWorld) prepareNetwork() error {
	w := s.w
	var err error
	s.controlDir, err = os.MkdirTemp(w.dir, "slice-14-controls-")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-14-probe"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(s.controlDir, "slice-14-probe"), b, 0755); err != nil {
		return err
	}
	if s.mode == "dns" || s.mode == "loss" {
		c, err := s.controlCreate(s.controlDir, []string{"/workspace/slice-14-probe", "resolver-plan"}, w.approval.uid+":"+w.approval.gid, "none", nil, nil, true)
		if err != nil {
			return err
		}
		r, err := s.controlWait(c)
		if err != nil {
			return err
		}
		if len(r) != 1 || r[0].Kind != "resolvers" {
			return errors.New("system resolver observation unavailable")
		}
		for _, line := range strings.Split(r[0].Values["resolv.conf"], "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && f[0] == "nameserver" {
				if net.ParseIP(f[1]) == nil {
					return errors.New("invalid system resolver target")
				}
				s.config.Resolvers = append(s.config.Resolvers, f[1])
			}
		}
		if len(s.config.Resolvers) == 0 {
			return errors.New("system resolver targets unavailable")
		}
	}
	s.networkName = "slice-14-network-" + isolationNonce()
	s.networkLabels = map[string]string{"slice-14.fixture": isolationNonce()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	yes := true
	n, err := s.cli.NetworkCreate(ctx, s.networkName, client.NetworkCreateOptions{Driver: "bridge", Internal: true, EnableIPv6: &yes, EnableIPv4: &yes, Labels: s.networkLabels, IPAM: &network.IPAM{Config: []network.IPAMConfig{{Subnet: netip.MustParsePrefix("172.30.14.0/24"), Gateway: netip.MustParseAddr("172.30.14.1")}, {Subnet: netip.MustParsePrefix("fd14:14::/64"), Gateway: netip.MustParseAddr("fd14:14::1")}}}})
	if err != nil {
		return err
	}
	s.networkID = n.ID
	switch s.mode {
	case "loss":
		err = s.lossRoutes()
	case "network":
		err = s.networkRoutes()
	default:
		err = s.dnsRoutes()
	}
	if err == nil && s.extraRoutes != nil {
		err = s.extraRoutes()
	}
	if err != nil {
		return err
	}
	controlConfig := s.config
	// Only the isolated control receives TLS private material. It is newly
	// generated test material, never an operator trust root or credential.
	if s.mode == "dns" || s.mode == "loss" {
		controlConfig.Cert, controlConfig.Key, err = isolationCertificate()
		if err != nil {
			return err
		}
		s.config.CA = controlConfig.Cert
		controlConfig.CA = controlConfig.Cert
		if s.mode == "loss" {
			// Keep the subject's nested DNS plan free of the control key too.
			s.config.DNS.CA = controlConfig.Cert
			dns := *s.config.DNS
			dns.Cert, dns.Key = controlConfig.Cert, controlConfig.Key
			controlConfig.DNS = &dns
		}
	}
	if err = os.WriteFile(filepath.Join(s.controlDir, "config.json"), approvalJSON(controlConfig), 0600); err != nil {
		return err
	}
	var aliases []string
	for _, nonce := range []string{s.config.Nonce + "b", s.config.Nonce, s.config.Nonce + "a"} {
		for _, phase := range []string{"first", "running"} {
			for _, route := range s.config.Routes {
				if route.Protocol == "docker" {
					aliases = append(aliases, nonce+"-"+phase+"-"+route.Name)
				}
			}
		}
	}
	s.control, err = s.controlCreate(s.controlDir, []string{"/workspace/slice-14-probe", "serve", "/workspace/config.json"}, "0:0", s.networkID, aliases, []string{"NET_ADMIN", "NET_RAW", "NET_BIND_SERVICE"}, true, func(h *container.HostConfig) {
		// Docker also makes its generated resolv.conf read-only with the root.
		// Only the DNS control rewrites its own resolver file to reproduce the
		// worker's observed targets; it needs a writable disposable root for that.
		h.ReadonlyRootfs = s.mode != "dns" && s.mode != "loss"
		// Host IPv6 defaults need not enable the control's loopback interface.
		// These settings belong only to its private namespace, never the worker
		// or runner. Keep every IPv6 route and require a usable dual-stack control.
		h.Sysctls = map[string]string{"net.ipv6.conf.all.disable_ipv6": "0", "net.ipv6.conf.default.disable_ipv6": "0", "net.ipv6.conf.lo.disable_ipv6": "0"}
	})
	if err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, err := s.controlLogs(s.control)
		if err != nil && !errors.Is(err, errIsolationNoReports) {
			return fmt.Errorf("listener/resolver control startup: %w", err)
		}
		if err == nil {
			for _, v := range r {
				if v.Kind == "ready" && v.Nonce == s.config.Nonce && v.Values["ipv6-loopback"] == "ready" {
					return nil
				}
			}
		}
		observed, err := s.controlInspect(s.control)
		if err != nil {
			return err
		}
		if observed.State == nil || !observed.State.Running {
			return errors.New("listener/resolver control exited before dual-stack readiness")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("isolated dual-stack listener/resolver control did not become ready")
}

func (s *isolationWorld) networkRoutes() error {
	// Host listeners bind ONLY to the fixture-owned internal bridge and
	// loopback. No public interface or real external destination is used.
	for name, address := range map[string]string{"gateway4": "172.30.14.1", "gateway6": "fd14:14::1", "host-loopback4": "127.0.0.1", "host-loopback6": "::1"} {
		port, err := s.hostEcho(address)
		if err != nil {
			return err
		}
		family := "4"
		if strings.Contains(address, ":") {
			family = "6"
		}
		for _, proto := range []string{"tcp", "udp"} {
			s.config.Routes = append(s.config.Routes, isolationRoute{Name: name + "-" + proto, Protocol: proto + family, Address: net.JoinHostPort(address, strconv.Itoa(port)), Answer: "host"})
		}
		if name == "host-loopback4" {
			s.config.Port = port
		}
	}
	// The loopback target is mirrored in the control namespace at the exact
	// host listener port. Its distinct identity prevents a worker-local success
	// being mistaken for reaching the host listener.
	for i := range s.config.Routes {
		if strings.HasPrefix(s.config.Routes[i].Name, "host-loopback") {
			s.config.Routes[i].Answer = "control"
		}
	}
	for name, address := range map[string]string{"literal4": "172.30.14.2", "literal6": "fd14:14::2", "private4": "10.14.0.2", "private6": "fd14:99::2", "metadata4": "169.254.169.254", "metadata6": "fd00:ec2::254"} {
		family := "4"
		if strings.Contains(address, ":") {
			family = "6"
		}
		if !strings.HasPrefix(name, "literal") {
			s.config.Addresses = append(s.config.Addresses, address)
		}
		for _, proto := range []string{"tcp", "udp"} {
			s.config.Routes = append(s.config.Routes, isolationRoute{Name: name + "-" + proto, Protocol: proto + family, Address: net.JoinHostPort(address, "32114"), Answer: "control"})
		}
	}
	return nil
}

func (s *isolationWorld) dnsRoutes() error {
	s.config.Addresses = append(s.config.Addresses, s.config.Resolvers...)
	s.config.Routes = []isolationRoute{{Name: "system-a", Protocol: "system4", Answer: "192.0.2.14"}, {Name: "system-aaaa", Protocol: "system6", Answer: "2001:db8::14"}, {Name: "docker-a", Protocol: "docker", Address: "127.0.0.11:53", Answer: "172.30.14.2"}, {Name: "docker-aaaa", Protocol: "docker", Address: "127.0.0.11:53", Answer: "fd14:14::2"}, {Name: "https-a", Protocol: "https", Address: "172.30.14.2:8443", Answer: "192.0.2.14"}}
	for name, address := range map[string]string{"direct4": "172.30.14.2", "direct6": "fd14:14::2", "loopback": "127.0.0.2"} {
		family := "4"
		if strings.Contains(address, ":") {
			family = "6"
		}
		for _, proto := range []string{"udp", "tcp"} {
			for _, kind := range []string{"a", "aaaa"} {
				answer := "192.0.2.14"
				if kind == "aaaa" {
					answer = "2001:db8::14"
				}
				s.config.Routes = append(s.config.Routes, isolationRoute{Name: name + "-" + proto + "-" + kind, Protocol: proto + family, Address: net.JoinHostPort(address, "53"), Answer: answer})
			}
		}
	}
	return nil
}

func isolationCertificate() (string, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(14), Subject: pkix.Name{CommonName: "slice-14 disposable resolver"}, DNSNames: []string{"slice-14.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})), nil
}

func (s *isolationWorld) hostEcho(address string) (int, error) {
	l, err := net.Listen("tcp", net.JoinHostPort(address, "0"))
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	p, err := net.ListenPacket("udp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		_ = l.Close()
		return 0, err
	}
	s.cleanup = append(s.cleanup, func() error { return errors.Join(l.Close(), p.Close()) })
	receipt := func(nonce string) { s.mu.Lock(); defer s.mu.Unlock(); s.receipts = append(s.receipts, nonce) }
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_ = c.SetDeadline(time.Now().Add(time.Second))
				v, err := bufio.NewReader(c).ReadString('\n')
				if err == nil {
					v = strings.TrimSpace(v)
					receipt(v)
					_, _ = io.WriteString(c, "host:"+v+"\n")
				}
			}()
		}
	}()
	go func() {
		b := make([]byte, 2048)
		for {
			n, a, err := p.ReadFrom(b)
			if err != nil {
				return
			}
			v := strings.TrimSpace(string(b[:n]))
			receipt(v)
			_, _ = p.WriteTo([]byte("host:"+v), a)
		}
	}()
	return port, nil
}

func (s *isolationWorld) routeControls(after bool) ([]isolationReport, error) {
	nonce := s.config.Nonce + "b"
	if after {
		nonce = s.config.Nonce + "a"
	}
	// Verify the actual host loopback listeners as well as their namespace
	// mirrors. Use the same Dial/write/read nonce exchange as the worker client.
	if s.mode == "network" {
		for _, route := range s.config.Routes {
			if !strings.HasPrefix(route.Name, "host-loopback") {
				continue
			}
			value := nonce + "-host-control-" + route.Name
			if err := isolationHostExchange(route, value); err != nil {
				return nil, err
			}
		}
	}
	var result []isolationReport
	for _, phase := range []string{"first", "running"} {
		r, err := s.controlExec(s.control, "0:0", "/workspace/slice-14-probe", "control", "/workspace/config.json", nonce, phase)
		if err != nil {
			return nil, err
		}
		result = append(result, r...)
	}
	return result, nil
}

func isolationHostExchange(route isolationRoute, value string) error {
	c, err := net.DialTimeout(route.Protocol, route.Address, 350*time.Millisecond)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(350 * time.Millisecond))
	if _, err = io.WriteString(c, value+"\n"); err != nil {
		return err
	}
	b := make([]byte, 1024)
	n, err := c.Read(b)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(b[:n])) != "host:"+value {
		return errors.New("actual host loopback listener did not echo the control nonce")
	}
	return nil
}
