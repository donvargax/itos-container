// Trusted CI endpoint. Built from this reviewed file, never from the disposable
// consumer checkout, and run only in a fixture container the harness owns. Its
// key material is generated here, in memory, and never leaves the process: the
// control directory receives certificates and receipts only.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// config is what the harness asks one endpoint to be. Redirect, when set, is
// the absolute URL every /redirect request is sent on to.
type config struct {
	Names    []string
	Nonce    string
	Listen   string
	Redirect string
}

// receipt is one thing the endpoint saw. A conn receipt is written when a TCP
// connection closes: how many bytes arrived on it, and whether TLS completed.
type receipt struct {
	Kind, Name, Nonce, Remote string
	Bytes                     int64
	Handshake                 bool
}

type recorder struct {
	mu   sync.Mutex
	path string
}

func (r *recorder) add(value receipt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, _ := json.Marshal(value)
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	_, _ = file.Write(append(data, '\n'))
	_ = file.Close()
}

type countedConn struct {
	net.Conn
	record *recorder
	mu     sync.Mutex
	bytes  int64
	shaken bool
	once   sync.Once
}

func (c *countedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.mu.Lock()
	c.bytes += int64(n)
	c.mu.Unlock()
	return n, err
}

func (c *countedConn) Close() error {
	c.once.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.record.add(receipt{Kind: "conn", Remote: c.RemoteAddr().String(), Bytes: c.bytes, Handshake: c.shaken})
	})
	return c.Conn.Close()
}

type countedListener struct {
	net.Listener
	record *recorder
}

func (l countedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &countedConn{Conn: conn, record: l.record}, nil
}

func certificates(names []string) (tls.Certificate, []byte, []byte, error) {
	serial := func() *big.Int { n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120)); return n }
	authorityKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	authority := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "slice-27 fixture authority"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	authorityDER, err := x509.CreateCertificate(rand.Reader, authority, authority, &authorityKey.PublicKey, authorityKey)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	leaf := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "slice-27 fixture endpoint"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		DNSNames: names, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, authority, &key.PublicKey, authorityKey)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	encode := func(der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}) }
	return tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: key}, encode(authorityDER), leafDER, nil
}

func serve(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return err
	}
	var cfg config
	if err = json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if cfg.Listen == "" {
		cfg.Listen = ":443"
	}
	certificate, authority, leaf, err := certificates(cfg.Names)
	if err != nil {
		return err
	}
	record := &recorder{path: filepath.Join(dir, "receipts.jsonl")}
	mux := http.NewServeMux()
	mux.HandleFunc("/nonce", func(out http.ResponseWriter, r *http.Request) {
		record.add(receipt{Kind: "nonce", Name: r.TLS.ServerName, Nonce: r.URL.Query().Get("n"), Remote: r.RemoteAddr})
		out.Header().Set("Connection", "close")
		_, _ = fmt.Fprint(out, cfg.Nonce)
	})
	mux.HandleFunc("/redirect", func(out http.ResponseWriter, r *http.Request) {
		record.add(receipt{Kind: "redirect", Name: r.TLS.ServerName, Nonce: r.URL.Query().Get("n"), Remote: r.RemoteAddr})
		out.Header().Set("Connection", "close")
		http.Redirect(out, r, cfg.Redirect, http.StatusFound)
	})
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}},
		ConnState: func(conn net.Conn, state http.ConnState) {
			// The server hands out its *tls.Conn; NetConn is the counted one.
			if secure, ok := conn.(*tls.Conn); ok && state == http.StateActive {
				if counted, ok := secure.NetConn().(*countedConn); ok {
					counted.mu.Lock()
					counted.shaken = true
					counted.mu.Unlock()
				}
			}
		}}
	sum := sha256.Sum256(leaf)
	public := map[string]string{"Address": listener.Addr().String(), "Leaf": hex.EncodeToString(sum[:])}
	ready, _ := json.Marshal(public)
	if err = os.WriteFile(filepath.Join(dir, "ca.pem"), authority, 0600); err != nil {
		return err
	}
	// Written last: its presence says the certificate is published and the port is open.
	if err = os.WriteFile(filepath.Join(dir, "ready.json"), ready, 0600); err != nil {
		return err
	}
	return server.ServeTLS(countedListener{listener, record}, "", "")
}

func main() {
	if len(os.Args) != 3 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: slice-27-endpoint serve <control directory>")
		os.Exit(2)
	}
	if err := serve(os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "slice-27 endpoint:", err)
		os.Exit(1)
	}
}
