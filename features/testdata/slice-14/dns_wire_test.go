package main

import (
	"encoding/hex"
	"testing"
)

func TestDNSAnswer(t *testing.T) {
	// Independent wire packets for the alias "control". Docker's local alias
	// replies repeat the owner name; the fixture resolver compresses it. Neither
	// encoding is made with dnsResponse, so its assumptions cannot hide a bug.
	for _, tt := range []struct {
		name, wire, want string
	}{
		{"compressed A", "14148180000100010000000007636f6e74726f6c0000010001c00c00010001000002580004ac1e0e02", "172.30.14.2"},
		{"Docker A", "14148180000100010000000007636f6e74726f6c000001000107636f6e74726f6c0000010001000002580004ac1e0e02", "172.30.14.2"},
		{"compressed AAAA", "14148180000100010000000007636f6e74726f6c00001c0001c00c001c0001000002580010fd140014000000000000000000000002", "fd14:14::2"},
		{"Docker AAAA", "14148180000100010000000007636f6e74726f6c00001c000107636f6e74726f6c00001c0001000002580010fd140014000000000000000000000002", "fd14:14::2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := hex.DecodeString(tt.wire)
			if err != nil {
				t.Fatal(err)
			}
			if got := dnsAnswer(packet); got != tt.want {
				t.Fatalf("answer = %q, want %q", got, tt.want)
			}
			for end := range len(packet) {
				if got := dnsAnswer(packet[:end]); got != "" {
					t.Fatalf("truncated at %d returned %q", end, got)
				}
			}
		})
	}
}

func TestDNSAnswerRejectsInvalidReplies(t *testing.T) {
	packet, err := hex.DecodeString("14148180000100010000000007636f6e74726f6c0000010001c00c00010001000002580004ac1e0e02")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		offset int
		value  byte
	}{
		{"different query ID", 0, 0},
		{"query instead of response", 2, 0x01},
		{"truncated response", 2, 0x83},
		{"NXDOMAIN", 3, 0x83},
		{"no question", 5, 0},
		{"no answer", 7, 0},
		{"non-IN question", 24, 3},
		{"compression loop", 26, 25},
		{"compression outside packet", 26, 255},
		{"reserved label encoding", 25, 0x40},
		{"different answer type", 28, 28},
		{"non-IN answer", 30, 3},
		{"wrong address length", 36, 16},
	} {
		t.Run(tt.name, func(t *testing.T) {
			invalid := append([]byte{}, packet...)
			invalid[tt.offset] = tt.value
			if got := dnsAnswer(invalid); got != "" {
				t.Fatalf("invalid reply returned %q", got)
			}
		})
	}
}

func TestDNSNameCompression(t *testing.T) {
	for _, tt := range []struct {
		name, wire string
		start, end int
		want       string
	}{
		{"literal prefix and pointer", "03666f6f0003626172c000", 5, 11, "bar.foo"},
		{"pointer through a pointer", "03666f6f00c000c005", 7, 9, "foo"},
		{"backward pointer cycle", "0161c000", 0, 0, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := hex.DecodeString(tt.wire)
			if err != nil {
				t.Fatal(err)
			}
			name, end := dnsName(packet, tt.start)
			if name != tt.want || end != tt.end {
				t.Fatalf("name/end = %q/%d, want %q/%d", name, end, tt.want, tt.end)
			}
		})
	}
}
