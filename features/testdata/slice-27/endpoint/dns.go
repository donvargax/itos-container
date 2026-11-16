package main

import (
	"encoding/binary"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Disposable authoritative control; no query is forwarded to an outside DNS
// service. Both transports use the already pinned x/net DNS codec.
func dnsReply(data []byte, names map[string]string, record *recorder) ([]byte, error) {
	var query dnsmessage.Message
	if err := query.Unpack(data); err != nil {
		return nil, err
	}
	reply := dnsmessage.Message{Header: dnsmessage.Header{ID: query.ID, Response: true, Authoritative: true, RecursionDesired: query.RecursionDesired, RecursionAvailable: true}, Questions: query.Questions}
	for _, q := range query.Questions {
		name := strings.TrimSuffix(q.Name.String(), ".")
		address := names[name]
		answer := address
		if address == "" {
			reply.RCode = dnsmessage.RCodeNameError
			answer = "nxdomain"
		}
		record.add(receipt{Kind: "dns", Name: name, Answer: answer})
		if ip := net.ParseIP(address).To4(); ip != nil && q.Type == dnsmessage.TypeA {
			var four [4]byte
			copy(four[:], ip)
			reply.Answers = append(reply.Answers, dnsmessage.Resource{Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}, Body: &dnsmessage.AResource{A: four}})
		}
	}
	return reply.Pack()
}

func serveDNS(names map[string]string, record *recorder) error {
	udp, err := net.ListenPacket("udp4", ":53")
	if err != nil {
		return err
	}
	tcp, err := net.Listen("tcp4", ":53")
	if err != nil {
		udp.Close()
		return err
	}
	go func() {
		defer udp.Close()
		data := make([]byte, 4096)
		for {
			n, peer, err := udp.ReadFrom(data)
			if err != nil {
				return
			}
			reply, err := dnsReply(data[:n], names, record)
			if err == nil {
				_, _ = udp.WriteTo(reply, peer)
			}
		}
	}()
	go func() {
		defer tcp.Close()
		for {
			conn, err := tcp.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				var size [2]byte
				if _, err := io.ReadFull(conn, size[:]); err != nil {
					return
				}
				data := make([]byte, int(binary.BigEndian.Uint16(size[:])))
				if _, err := io.ReadFull(conn, data); err != nil {
					return
				}
				reply, err := dnsReply(data, names, record)
				if err != nil {
					return
				}
				binary.BigEndian.PutUint16(size[:], uint16(len(reply)))
				_, _ = conn.Write(append(size[:], reply...))
			}()
		}
	}()
	return nil
}
