package tcp

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"math/big"
	"net"
	"testing"
	"time"
)

// clientHello captures the first flight of Go's own TLS client, in memory over a
// pipe, so the hello is real and no capture enters the tree.
func clientHello(t testing.TB, config *tls.Config) []byte {
	t.Helper()
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	go func() {
		_ = tls.Client(client, config).Handshake()
		_ = client.Close()
	}()
	buf := make([]byte, 1<<16)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return buf[:n]
}

// helloFor is Go's hello for a server name, with no verification asked of it.
func helloFor(t testing.TB, name string) []byte {
	t.Helper()
	return clientHello(t, &tls.Config{ServerName: name, InsecureSkipVerify: true})
}

// echHello is Go's hello with encrypted_client_hello, from a config list holding
// a real X25519 key. Its server name is the config's public name.
func echHello(t testing.TB, publicName string) []byte {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var config []byte
	config = append(config, 0xfe, 0x0d)
	body := []byte{7, 0x00, 0x20, 0x00, 0x20}
	body = append(body, key.PublicKey().Bytes()...)
	body = append(body, 0x00, 0x04, 0x00, 0x01, 0x00, 0x01, 32, byte(len(publicName)))
	body = append(body, publicName...)
	body = append(body, 0x00, 0x00)
	config = binary.BigEndian.AppendUint16(config, uint16(len(body)))
	config = append(config, body...)
	list := binary.BigEndian.AppendUint16(nil, uint16(len(config)))
	list = append(list, config...)
	return clientHello(t, &tls.Config{ServerName: "listed.example", InsecureSkipVerify: true,
		EncryptedClientHelloConfigList: list})
}

// withExtension appends one extension to a single-record hello and fixes the
// three lengths above it: the extensions', the handshake message's and the record's.
func withExtension(t testing.TB, hello []byte, id uint16, body []byte) []byte {
	t.Helper()
	// record header 5, handshake header 4, version 2, random 32, then three
	// length-prefixed vectors, then the extensions.
	at := 5 + 4 + 2 + 32
	at += 1 + int(hello[at])
	at += 2 + int(binary.BigEndian.Uint16(hello[at:]))
	at += 1 + int(hello[at])
	extensions := binary.BigEndian.Uint16(hello[at:])
	if at+2+int(extensions) != len(hello) {
		t.Fatalf("hello is not one record ending in its extensions")
	}
	out := append([]byte(nil), hello...)
	out = binary.BigEndian.AppendUint16(out, id)
	out = binary.BigEndian.AppendUint16(out, uint16(len(body)))
	out = append(out, body...)
	grow := 4 + len(body)
	binary.BigEndian.PutUint16(out[at:], extensions+uint16(grow))
	handshake := int(out[6])<<16 | int(out[7])<<8 | int(out[8])
	handshake += grow
	out[6], out[7], out[8] = byte(handshake>>16), byte(handshake>>8), byte(handshake)
	binary.BigEndian.PutUint16(out[3:], binary.BigEndian.Uint16(out[3:])+uint16(grow))
	return out
}

// upstreamCertificate makes a certificate for name in memory, so no key enters a file.
func upstreamCertificate(t testing.TB, name string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
