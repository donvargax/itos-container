// Trusted CI probe. Built from this reviewed file, never from the disposable
// consumer checkout. It runs only as the workload of a worker the product
// created, follows the plan the harness wrote beside it and records what each
// step observed. It holds no key: its trust is the fixture's public certificate.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type step struct {
	Op, Name, Mode, Method, Target, Input, Nonce, URL string
}

type plan struct {
	Steps []step
	Exit  int
}

type result struct {
	Op, Name, Mode, Method, Target, Input string
	OK, Closed                            bool
	Status, Body, Received                int
	Peer, Reply, Location, Error          string
}

type report struct {
	Env     map[string]string
	Results []result
	Done    bool
}

const wait = 20 * time.Second

// brief bounds one attempt that is expected to go nowhere: a route the worker
// must not have. A reachable route answers well inside it.
const brief = 2 * time.Second

func proxyAddress() (string, error) {
	value := os.Getenv("HTTPS_PROXY")
	if value == "" {
		value = os.Getenv("https_proxy")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", errors.New("no proxy variable")
	}
	return parsed.Host, nil
}

// head reads one response head byte by byte, so nothing after it is consumed.
func head(conn net.Conn) (status int, raw []byte, err error) {
	one := make([]byte, 1)
	for !bytes.HasSuffix(raw, []byte("\r\n\r\n")) {
		if len(raw) > 8192 {
			return 0, raw, errors.New("response head too long")
		}
		if _, err = io.ReadFull(conn, one); err != nil {
			return 0, raw, err
		}
		raw = append(raw, one[0])
	}
	if _, err = fmt.Sscanf(string(raw), "HTTP/1.1 %d", &status); err != nil {
		return 0, raw, errors.New("no HTTP/1.1 status line")
	}
	return status, raw, nil
}

func connect(target string) (net.Conn, int, error) {
	address, err := proxyAddress()
	if err != nil {
		return nil, 0, err
	}
	conn, err := net.DialTimeout("tcp", address, wait)
	if err != nil {
		return nil, 0, err
	}
	_ = conn.SetDeadline(time.Now().Add(wait))
	if _, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target); err != nil {
		_ = conn.Close()
		return nil, 0, err
	}
	status, _, err := head(conn)
	if err != nil {
		_ = conn.Close()
		return nil, 0, err
	}
	return conn, status, nil
}

func fingerprint(state *tls.ConnectionState) string {
	if state == nil || len(state.PeerCertificates) == 0 {
		return ""
	}
	sum := sha256.Sum256(state.PeerCertificates[0].Raw)
	return hex.EncodeToString(sum[:])
}

func client(roots *x509.CertPool) *http.Client {
	return &http.Client{Timeout: wait, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}}
}

func exchange(s step, roots *x509.CertPool, r *result) error {
	if s.Mode == "env" {
		response, err := client(roots).Get("https://" + s.Name + "/nonce?n=" + url.QueryEscape(s.Nonce))
		if err != nil {
			return err
		}
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		r.Status, r.Reply, r.Peer = response.StatusCode, string(body), fingerprint(response.TLS)
		return err
	}
	conn, status, err := connect(s.Name + ":443")
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	r.Status = status
	if status != http.StatusOK {
		return errors.New("tunnel refused")
	}
	secure := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: s.Name})
	if err = secure.Handshake(); err != nil {
		return err
	}
	state := secure.ConnectionState()
	r.Peer = fingerprint(&state)
	if _, err = fmt.Fprintf(secure, "GET /nonce?n=%s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", url.QueryEscape(s.Nonce), s.Name); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(secure, 8192))
	if at := bytes.Index(data, []byte("\r\n\r\n")); at >= 0 {
		r.Reply = string(data[at+4:])
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || r.Reply != "" {
		err = nil
	}
	return err
}

// raw sends one request line exactly as written, so no client library can
// respell its target, and reads to the end of the connection.
func raw(s step, r *result) error {
	address, err := proxyAddress()
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", address, wait)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(wait))
	if _, err = fmt.Fprintf(conn, "%s %s HTTP/1.1\r\nHost: %s\r\n\r\n", s.Method, s.Target, s.Target); err != nil {
		return err
	}
	status, _, err := head(conn)
	if err != nil {
		return err
	}
	r.Status = status
	body, err := io.ReadAll(conn)
	r.Body, r.Closed = len(body), err == nil
	return nil
}

// clientHello is one TLS 1.2 record holding a ClientHello with the extensions
// asked for. It is input for a parser; no handshake is ever completed with it.
func clientHello(name string, ech bool) []byte {
	u16 := func(n int) []byte { return []byte{byte(n >> 8), byte(n)} }
	var extensions []byte
	if name != "" {
		entry := append([]byte{0}, append(u16(len(name)), name...)...)
		list := append(u16(len(entry)), entry...)
		extensions = append(extensions, append([]byte{0, 0}, append(u16(len(list)), list...)...)...)
	}
	extensions = append(extensions, 0, 43, 0, 3, 2, 3, 4) // supported_versions: TLS 1.3
	if ech {
		// A well-formed outer ECHClientHello: HKDF-SHA256 with AES-128-GCM, a
		// config ID, a 32-byte encapsulated key and a 16-byte payload.
		payload := []byte{0, 0, 1, 0, 1, 0x27, 0, 32}
		payload = append(payload, bytes.Repeat([]byte{0x27}, 32)...)
		payload = append(append(payload, 0, 16), bytes.Repeat([]byte{0x27}, 16)...)
		extensions = append(extensions, append([]byte{0xfe, 0x0d}, append(u16(len(payload)), payload...)...)...)
	}
	body := []byte{3, 3}
	body = append(body, bytes.Repeat([]byte{0x27}, 32)...)
	body = append(body, 0)                // no session ID
	body = append(body, 0, 2, 0x13, 0x01) // one cipher suite
	body = append(body, 1, 0)             // null compression
	body = append(body, u16(len(extensions))...)
	body = append(body, extensions...)
	handshake := append([]byte{1, 0}, append(u16(len(body)), body...)...)
	return append([]byte{22, 3, 1}, append(u16(len(handshake)), handshake...)...)
}

func helloInput(s step) (data []byte, half bool, err error) {
	switch {
	case s.Input == "ClientHello without SNI":
		return clientHello("", false), false, nil
	case s.Input == "ClientHello with ECH extension 0xfe0d":
		return clientHello(s.Name, true), false, nil
	case s.Input == "plain HTTP bytes":
		return []byte("GET / HTTP/1.1\r\nHost: " + s.Name + "\r\n\r\n"), false, nil
	case s.Input == "an SSH banner":
		return []byte("SSH-2.0-OpenSSH_9.9\r\n"), false, nil
	case s.Input == "a truncated ClientHello followed by EOF":
		whole := clientHello(s.Name, false)
		return whole[:len(whole)/2], true, nil
	case strings.HasPrefix(s.Input, "ClientHello with ") && strings.HasSuffix(s.Input, " SNI"):
		return clientHello(strings.TrimSuffix(strings.TrimPrefix(s.Input, "ClientHello with "), " SNI"), false), false, nil
	}
	return nil, false, errors.New("unknown input")
}

func hello(s step, r *result) error {
	data, half, err := helloInput(s)
	if err != nil {
		return err
	}
	conn, status, err := connect(s.Name + ":443")
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	r.Status = status
	if status != http.StatusOK {
		return errors.New("tunnel refused")
	}
	if _, err = conn.Write(data); err != nil {
		return err
	}
	if tcp, ok := conn.(*net.TCPConn); ok && half {
		_ = tcp.CloseWrite()
	}
	received, err := io.ReadAll(conn)
	r.Received, r.Closed = len(received), err == nil
	return nil
}

// listed is the plain NO_PROXY rule every common client shares: an entry names
// a host exactly. Go's own transport never proxies loopback whatever the
// variables say, so it could not show that the variables are what keeps a
// local service local.
func listed(host string) bool {
	for _, variable := range []string{"NO_PROXY", "no_proxy"} {
		found := false
		for _, entry := range strings.Split(os.Getenv(variable), ",") {
			found = found || strings.TrimSpace(entry) == host
		}
		if !found {
			return false
		}
	}
	return true
}

func loopback(s step, r *result) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{ReadHeaderTimeout: wait, Handler: http.HandlerFunc(func(out http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(out, s.Nonce) })}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	for _, host := range []string{"127.0.0.1", "localhost"} {
		// A host the variables do not exempt goes to the proxy, as a client
		// that honours them would send it, and the proxy refuses it.
		transport := &http.Transport{DisableKeepAlives: true, Proxy: func(*http.Request) (*url.URL, error) {
			if listed(host) {
				return nil, nil
			}
			return url.Parse(os.Getenv("HTTP_PROXY"))
		}}
		response, err := (&http.Client{Timeout: wait, Transport: transport}).Get("http://" + net.JoinHostPort(host, port) + "/")
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || string(body) != s.Nonce {
			return fmt.Errorf("%s answered %d", host, response.StatusCode)
		}
	}
	r.Reply = s.Nonce
	return nil
}

func redirect(s step, roots *x509.CertPool, r *result) error {
	c := client(roots)
	c.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		if r.Location == "" && next.Response != nil {
			r.Status, r.Location, r.Peer = next.Response.StatusCode, next.Response.Header.Get("Location"), fingerprint(next.Response.TLS)
		}
		return nil
	}
	response, err := c.Get(s.URL)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	r.Reply = fmt.Sprint(response.StatusCode)
	return nil
}

// isolation runs slice-14's trusted route and resolver client beside this
// probe, once for the named phase, and keeps its framed report. That client
// dials every planned destination itself and reads no proxy variable.
func isolation(dir string, s step, r *result) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*wait)
	defer cancel()
	var out, problems bytes.Buffer
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "slice-14-probe"), "control", s.Target, s.Nonce, s.Name)
	cmd.Stdout, cmd.Stderr = &out, &problems
	err := cmd.Run()
	r.Reply = out.String()
	if err != nil {
		return fmt.Errorf("route client: %w: %s", err, problems.String())
	}
	return nil
}

// line is one nonce exchange with a plain echo service: the nonce and a
// newline out, one read back. A send that nothing answers is not an exchange.
func line(protocol, address, nonce string) (string, error) {
	conn, err := net.DialTimeout(protocol, address, brief)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(brief))
	if _, err = io.WriteString(conn, nonce+"\n"); err != nil {
		return "", err
	}
	reply := make([]byte, 4096)
	n, err := conn.Read(reply)
	if n == 0 && err == nil {
		err = errors.New("no reply")
	}
	return strings.TrimSpace(string(reply[:n])), err
}

// direct ignores the proxy variables: it dials the address itself and, only if
// that connects, completes the same verified nonce exchange a tunnel carries.
func direct(s step, roots *x509.CertPool, r *result) error {
	conn, err := net.DialTimeout(s.Method, s.Target, brief)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(brief))
	secure := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: s.Name})
	if err = secure.Handshake(); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	state := secure.ConnectionState()
	r.Peer = fingerprint(&state)
	if _, err = fmt.Fprintf(secure, "GET /nonce?n=%s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", url.QueryEscape(s.Nonce), s.Name); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(secure, 8192))
	if at := bytes.Index(data, []byte("\r\n\r\n")); at >= 0 {
		r.Reply = string(data[at+4:])
	}
	if r.Reply != "" {
		return nil
	}
	return fmt.Errorf("no reply: %v", err)
}

// forward treats the proxy's own address as a plain forwarder: a nonce
// exchange with another of its ports, with no CONNECT anywhere.
func forward(s step, r *result) error {
	address, err := proxyAddress()
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	r.Reply, err = line(s.Method, net.JoinHostPort(host, s.Target), s.Nonce)
	if err == nil && r.Reply == "" {
		err = errors.New("no reply")
	}
	return err
}

// quicInitial is a datagram shaped as a QUIC version 1 Initial: long header,
// version, connection IDs, an empty token and a length, padded to the 1200
// bytes a client's first flight must fill. Its payload is the nonce in the
// clear; no handshake exists behind it.
func quicInitial(nonce string) []byte {
	packet := []byte{0xc3, 0, 0, 0, 1, 8}
	packet = append(packet, bytes.Repeat([]byte{0x27}, 8)...)
	packet = append(packet, 0, 0) // no source connection ID, no token
	length := 1200 - len(packet) - 2
	packet = append(packet, 0x40|byte(length>>8), byte(length))
	packet = append(packet, nonce...)
	return append(packet, make([]byte, 1200-len(packet))...)
}

func datagram(s step, r *result) error {
	conn, err := net.DialTimeout(s.Method, s.Target, brief)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(brief))
	if _, err = conn.Write(quicInitial(s.Nonce)); err != nil {
		return err
	}
	reply := make([]byte, 4096)
	n, err := conn.Read(reply)
	r.Received = n
	// The echo returns the datagram behind its own identity; only that and
	// whether the nonce came back are kept.
	if identity, rest, found := bytes.Cut(reply[:n], []byte(":")); found && bytes.Contains(rest, []byte(s.Nonce)) {
		r.Reply = string(identity) + ":" + s.Nonce
	}
	if r.Reply == "" && err == nil {
		err = errors.New("no echo of the nonce")
	}
	return err
}

// lookup asks the resolver at Target for Name over the protocol Mode names,
// as an absolute name so no search list is tried. The Go resolver does the
// DNS framing; only the transport is chosen here.
func lookup(s step, r *result) error {
	protocol := "udp4"
	if s.Mode == "docker-tcp" {
		protocol = "tcp4"
	}
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: brief}).DialContext(ctx, protocol, s.Target)
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*brief)
	defer cancel()
	addresses, err := resolver.LookupIP(ctx, s.Input, s.Name+".")
	if len(addresses) != 0 {
		r.Reply = addresses[0].String()
		return nil
	}
	if err == nil {
		err = errors.New("no answer")
	}
	return err
}

// routes reports the worker's own IPv4 routing table as the kernel prints it.
func routes(r *result) error {
	data, err := os.ReadFile("/proc/net/route")
	r.Reply = string(data)
	return err
}

func barrier(dir string, s step) error {
	if err := os.WriteFile(filepath.Join(dir, "slice-27-barrier-"+s.Name), nil, 0600); err != nil {
		return err
	}
	for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(dir, "slice-27-release-"+s.Name)); err == nil {
			return nil
		}
	}
	return errors.New("never released")
}

func save(dir string, value report) {
	data, _ := json.Marshal(value)
	temporary := filepath.Join(dir, "slice-27-report.tmp")
	if os.WriteFile(temporary, data, 0600) == nil {
		_ = os.Rename(temporary, filepath.Join(dir, "slice-27-report.json"))
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: slice-27-probe <plan>")
		os.Exit(96)
	}
	dir := filepath.Dir(os.Args[1])
	data, err := os.ReadFile(os.Args[1])
	var p plan
	if err != nil || json.Unmarshal(data, &p) != nil {
		fmt.Fprintln(os.Stderr, "slice-27 probe: unreadable plan")
		os.Exit(96)
	}
	roots := x509.NewCertPool()
	if authority, err := os.ReadFile(filepath.Join(dir, "slice-27-ca.pem")); err == nil {
		roots.AppendCertsFromPEM(authority)
	}
	out := report{Env: map[string]string{}}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "NO_PROXY", "no_proxy", "ALL_PROXY", "all_proxy"} {
		if value, ok := os.LookupEnv(key); ok {
			out.Env[key] = value
		}
	}
	for _, s := range p.Steps {
		r := result{Op: s.Op, Name: s.Name, Mode: s.Mode, Method: s.Method, Target: s.Target, Input: s.Input}
		var err error
		switch s.Op {
		case "barrier":
			err = barrier(dir, s)
		case "tls":
			err = exchange(s, roots, &r)
		case "raw":
			err = raw(s, &r)
		case "hello":
			err = hello(s, &r)
		case "loopback":
			err = loopback(s, &r)
		case "redirect":
			err = redirect(s, roots, &r)
		case "isolation":
			err = isolation(dir, s, &r)
		case "direct":
			err = direct(s, roots, &r)
		case "forward":
			err = forward(s, &r)
		case "datagram":
			err = datagram(s, &r)
		case "lookup":
			err = lookup(s, &r)
		case "routes":
			err = routes(&r)
		default:
			err = errors.New("unknown step")
		}
		r.OK = err == nil
		if err != nil {
			r.Error = err.Error()
		}
		out.Results = append(out.Results, r)
		save(dir, out)
	}
	out.Done = true
	save(dir, out)
	os.Exit(p.Exit)
}
