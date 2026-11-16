package features

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// egressTestProxy is a stand-in for the product's proxy in this harness control
// only: it tunnels two names to the endpoint, hands "peek.example" to the
// standard library's own ClientHello parser, and refuses everything else.
type egressTestProxy struct {
	listener net.Listener
	upstream string
	mu       sync.Mutex
	peeked   []string
}

func (p *egressTestProxy) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.handle(conn)
	}
}

func (p *egressTestProxy) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	switch {
	case request.Method == http.MethodConnect && (request.Host == "api.example.com:443" || request.Host == "download.example:443"):
		upstream, err := net.Dial("tcp", p.upstream)
		if err != nil {
			return
		}
		defer func() { _ = upstream.Close() }()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(upstream, reader); _ = upstream.(*net.TCPConn).CloseWrite() }()
		_, _ = io.Copy(conn, upstream)
	case request.Method == http.MethodConnect && request.Host == "peek.example:443":
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = tls.Server(conn, &tls.Config{MinVersion: tls.VersionTLS12, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			p.mu.Lock()
			p.peeked = append(p.peeked, hello.ServerName+"|"+fmt.Sprint(slices.Contains(hello.Extensions, 0xfe0d)))
			p.mu.Unlock()
			return nil, errors.New("parsed; no handshake is wanted")
		}}).Handshake()
	default:
		_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
	}
}

// The two trusted programs of the real-engine scenarios are exercised here
// against each other, with no engine and no product: what the probe reports and
// what the endpoint receipts must mean what the native assertions read.
func TestEgressTrustedProgramsExchangeWithoutAnEngine(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("the trusted programs are built for Linux amd64")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	programs, control, checkout := t.TempDir(), t.TempDir(), t.TempDir()
	for _, name := range []string{"endpoint", "probe"} {
		if err := egressBuild(root, filepath.Join("features", "testdata", "slice-27", name), filepath.Join(programs, name)); err != nil {
			t.Fatal(err)
		}
	}
	redirect := "https://unlisted.example/nonce?n=redirected"
	config := map[string]any{"Names": []string{"api.example.com", "download.example"}, "Nonce": "endpoint-nonce", "Listen": "127.0.0.1:0", "Redirect": redirect}
	if err := os.WriteFile(filepath.Join(control, "config.json"), approvalJSON(config), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	endpoint := exec.CommandContext(ctx, filepath.Join(programs, "endpoint"), "serve", control)
	if err := endpoint.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = endpoint.Wait() })
	var ready struct{ Address, Leaf string }
	for ready.Leaf == "" {
		if data, err := os.ReadFile(filepath.Join(control, "ready.json")); err == nil {
			_ = json.Unmarshal(data, &ready)
		}
		if ctx.Err() != nil {
			t.Fatal("the endpoint never published its certificate")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, file := range []string{"ca.pem", "ready.json", "config.json"} {
		if data, _ := os.ReadFile(filepath.Join(control, file)); strings.Contains(string(data), "PRIVATE KEY") {
			t.Fatalf("%s carries key material", file)
		}
	}
	authority, err := os.ReadFile(filepath.Join(control, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(checkout, "slice-27-ca.pem"), authority, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	proxy := &egressTestProxy{listener: listener, upstream: ready.Address}
	go proxy.serve()
	inputs := []string{"ClientHello with other.example SNI", "ClientHello with API.example.com SNI", "ClientHello without SNI", "ClientHello with ECH extension 0xfe0d"}
	steps := []egressStep{{Op: "barrier", Name: "attach"},
		{Op: "tls", Mode: "env", Name: "api.example.com", Nonce: "env-nonce"}, {Op: "tls", Mode: "connect", Name: "api.example.com", Nonce: "connect-nonce"},
		{Op: "raw", Method: "CONNECT", Target: "other.example:443"}, {Op: "raw", Method: "GET", Target: "http://api.example.com/path"},
		{Op: "loopback", Nonce: "loopback-nonce"}, {Op: "redirect", URL: "https://download.example/redirect?n=worker-nonce"}}
	for _, input := range inputs {
		steps = append(steps, egressStep{Op: "hello", Name: "peek.example", Input: input})
	}
	steps = append(steps, egressStep{Op: "hello", Name: "peek.example", Input: "an SSH banner"}, egressStep{Op: "hello", Name: "unknown.example", Input: "plain HTTP bytes"})
	if err = os.WriteFile(filepath.Join(checkout, "slice-27-plan.json"), approvalJSON(map[string]any{"Steps": steps, "Exit": 7}), 0600); err != nil {
		t.Fatal(err)
	}
	probe := exec.CommandContext(ctx, filepath.Join(programs, "probe"), filepath.Join(checkout, "slice-27-plan.json"))
	address := "http://" + listener.Addr().String()
	probe.Env = []string{"HTTPS_PROXY=" + address, "HTTP_PROXY=" + address, "https_proxy=" + address, "http_proxy=" + address, "NO_PROXY=localhost,127.0.0.1,::1", "no_proxy=localhost,127.0.0.1,::1"}
	if err = probe.Start(); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(checkout, "slice-27-barrier-attach")); err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("the probe never reached its barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(checkout, "slice-27-report.json")); err == nil {
		t.Fatal("the probe reported before its barrier was released")
	}
	if err = os.WriteFile(filepath.Join(checkout, "slice-27-release-attach"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	var exit *exec.ExitError
	if err = probe.Wait(); !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("the probe did not return the planned exit: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(checkout, "slice-27-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	run := &egressNativeRun{}
	if err = json.Unmarshal(data, &run.report); err != nil || !run.report.Done || len(run.report.Results) != len(steps) {
		t.Fatalf("incomplete report: %v\n%s", err, data)
	}
	for _, r := range run.results("tls") {
		if !r.OK || r.Status != 200 || r.Peer != ready.Leaf || r.Reply != "endpoint-nonce" {
			t.Errorf("TLS exchange %s: %+v", r.Mode, r)
		}
	}
	for _, r := range run.results("raw") {
		if !r.OK || r.Status != 403 || r.Body != 0 || !r.Closed {
			t.Errorf("raw request %s %s: %+v", r.Method, r.Target, r)
		}
	}
	if r := run.results("loopback"); len(r) != 1 || !r[0].OK {
		t.Errorf("loopback: %+v", r)
	}
	if r := run.results("redirect"); len(r) != 1 || r[0].OK || r[0].Status != 302 || r[0].Location != redirect || r[0].Peer != ready.Leaf || !strings.Contains(r[0].Error, "Forbidden") {
		t.Errorf("redirect: %+v", r)
	}
	hellos := run.results("hello")
	for i, r := range hellos {
		want := 200
		if i == len(hellos)-1 {
			want = 403
		}
		if r.Status != want {
			t.Errorf("hello %q: tunnel status %d", r.Input, r.Status)
		}
	}
	proxy.mu.Lock()
	peeked := slices.Clone(proxy.peeked)
	proxy.mu.Unlock()
	if !slices.Equal(peeked, []string{"other.example|false", "API.example.com|false", "|false", "peek.example|true"}) {
		t.Errorf("the standard library read the crafted hellos as %v", peeked)
	}
	e := &egressEndpoint{key: "local", dir: control}
	if err = e.onlyExchanges(false); err != nil {
		t.Error(err)
	}
	receipts, _ := e.receipts()
	for _, want := range []egressReceipt{{Kind: "nonce", Name: "api.example.com", Nonce: "env-nonce"}, {Kind: "nonce", Name: "api.example.com", Nonce: "connect-nonce"}, {Kind: "redirect", Name: "download.example", Nonce: "worker-nonce"}} {
		if !slices.ContainsFunc(receipts, func(r egressReceipt) bool { return r.Kind == want.Kind && r.Name == want.Name && r.Nonce == want.Nonce }) {
			t.Errorf("no receipt %+v in %+v", want, receipts)
		}
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		if run.report.Env[key] != address {
			t.Errorf("the probe reported %s=%q", key, run.report.Env[key])
		}
	}
}
