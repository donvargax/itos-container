package main

import (
	"encoding/binary"
	"net"
	"strings"
)

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
	name, end := dnsName(b, 12)
	if end == 0 || end+4 > len(b) {
		return "", 0, 0
	}
	return name, end + 4, binary.BigEndian.Uint16(b[end:])
}

// A name can be literal, compressed, or a literal prefix followed by a pointer.
// Return the end of its on-wire encoding, not a fixed two-byte pointer offset.
func dnsName(b []byte, offset int) (string, int) {
	var labels []string
	end := 0
	for remaining := len(b); remaining > 0 && offset < len(b); remaining-- {
		n := int(b[offset])
		switch {
		case n == 0:
			if end == 0 {
				end = offset + 1
			}
			return strings.Join(labels, "."), end
		case n&0xc0 == 0xc0:
			if offset+1 >= len(b) {
				return "", 0
			}
			target := int(binary.BigEndian.Uint16(b[offset:]) & 0x3fff)
			if target >= offset {
				return "", 0
			}
			if end == 0 {
				end = offset + 2
			}
			offset = target
		case n <= 63 && offset+1+n <= len(b):
			labels = append(labels, string(b[offset+1:offset+1+n]))
			offset += 1 + n
		default:
			return "", 0
		}
	}
	return "", 0
}

func dnsAnswer(b []byte) string {
	name, end, kind := question(b)
	if end == 0 || binary.BigEndian.Uint16(b[4:]) != 1 || binary.BigEndian.Uint16(b[6:]) < 1 || binary.BigEndian.Uint16(b[end-2:]) != 1 {
		return ""
	}
	// Only an untruncated successful standard response to our query counts.
	if binary.BigEndian.Uint16(b) != 0x1414 || binary.BigEndian.Uint16(b[2:])&0xfa0f != 0x8000 {
		return ""
	}
	owner, end := dnsName(b, end)
	if end == 0 || end+10 > len(b) || !strings.EqualFold(owner, name) || binary.BigEndian.Uint16(b[end:]) != kind || binary.BigEndian.Uint16(b[end+2:]) != 1 {
		return ""
	}
	n := int(binary.BigEndian.Uint16(b[end+8:]))
	if end+10+n > len(b) || !((kind == 1 && n == 4) || (kind == 28 && n == 16)) {
		return ""
	}
	return net.IP(b[end+10 : end+10+n]).String()
}
