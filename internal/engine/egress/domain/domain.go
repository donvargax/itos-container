// Package domain decides what the egress proxy admits: the allowlist, the CONNECT
// head, the ClientHello, and the numbering of the refusal log. It is pure: the
// adapter reads the bytes and carries out the verdicts. A verdict refuses at its
// zero value, so a forgotten branch fails closed.
package domain

import (
	"bytes"
	"slices"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// The limits of one connection and of one run.
const (
	// HeadLimit is the most bytes of CONNECT head the proxy reads.
	HeadLimit = 8192
	// HelloLimit is the most bytes the ClientHello peek reads.
	HelloLimit = 65536
	// LogLimit is how many refused or upstream-failed records end a run.
	LogLimit = 20000
)

const (
	echExtension = 0xfe0d
	maxName      = 253
	maxLabel     = 63
	maxHost      = 255
	maxMethod    = 16
	maxPort      = 5
	crlf         = "\r\n"
	terminator   = crlf + crlf
)

// Allowlist is a validated list of canonical names. Its zero value lists nothing.
type Allowlist struct{ names map[string]bool }

// NewAllowlist validates the list: it must be non-empty, name nothing twice, and
// hold only canonical names. A name that is not canonical is refused, never mapped.
func NewAllowlist(names []string) (Allowlist, bool) {
	if len(names) == 0 {
		return Allowlist{}, false
	}
	set := make(map[string]bool, len(names))
	for _, name := range names {
		if set[name] || !canonicalName(name) {
			return Allowlist{}, false
		}
		set[name] = true
	}
	return Allowlist{names: set}, true
}

// canonicalName is the shape every list the policy produces has: at least two
// labels of 1 to 63 bytes of a-z, 0-9 and hyphen, at most 253 bytes in all, no
// trailing dot, and a final label no address parser would read as a number.
func canonicalName(name string) bool {
	if len(name) > maxName {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > maxLabel || strings.Trim(label, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			return false
		}
	}
	return !addressLike(labels[len(labels)-1])
}

// addressLike is whether a final label is all decimal digits, or 0x followed by
// only hexadecimal digits, which address parsers read as a number.
func addressLike(label string) bool {
	if hex, ok := strings.CutPrefix(label, "0x"); ok {
		return strings.Trim(hex, "0123456789abcdef") == ""
	}
	return strings.Trim(label, "0123456789") == ""
}

// Head is the verdict on what the client sent first. Its zero value refuses.
// Method, Target and Port hold only what the log may write.
type Head struct {
	Pass   bool
	Reason port.Reason
	Method string
	Target string
	Port   string
	// Tunnel is where the bytes after the head's terminator start in the input.
	Tunnel int
}

// HeadEnd returns the offset just after the first CRLF CRLF within the first
// HeadLimit bytes of input. A terminator ending at exactly HeadLimit counts.
func HeadEnd(input []byte) (int, bool) {
	index := bytes.Index(input[:min(len(input), HeadLimit)], []byte(terminator))
	if index < 0 {
		return 0, false
	}
	return index + len(terminator), true
}

// Judge decides on the CONNECT head at the start of input. A request passes only
// when its method is exactly CONNECT and its target is exactly a listed name
// followed by :443; anything else is refused with the first reason that fits.
func (a Allowlist) Judge(input []byte) Head {
	end, ok := HeadEnd(input)
	if !ok {
		return Head{Reason: port.Malformed}
	}
	line := string(input[:bytes.Index(input[:end], []byte(crlf))])
	parts := strings.Split(line, " ")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(line, "\r\n") ||
		(parts[2] != "HTTP/1.1" && parts[2] != "HTTP/1.0") {
		return Head{Reason: port.Malformed}
	}
	method, target := parts[0], parts[1]
	host, portText, plain := splitTarget(target)
	head := Head{Tunnel: end}
	if keptMethod(method) {
		head.Method = method
	}
	if plain {
		head.Target, head.Port = host, portText
	}
	switch {
	case method != "CONNECT":
		head.Reason = port.NotConnect
	case !plain:
		head.Reason = port.Malformed
	case literal(host):
		head.Reason = port.LiteralIP
	case !a.names[host] && a.names[strings.ToLower(strings.TrimSuffix(host, "."))]:
		head.Reason = port.NotCanonical
	case !a.names[host]:
		head.Reason = port.NotListed
	case portText != "443":
		head.Reason = port.Port
	default:
		head.Pass = true
	}
	return head
}

// keptMethod is whether the log may write a method: 1 to 16 ASCII capitals.
func keptMethod(method string) bool {
	return method != "" && len(method) <= maxMethod && strings.Trim(method, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") == ""
}

// splitTarget splits at the last colon. It reports plain only for a host of 1 to
// 255 bytes, ASCII letters, digits, hyphen, underscore and dot or a bracketed
// IPv6 literal, and a port of 1 to 5 digits.
func splitTarget(target string) (host, portText string, plain bool) {
	colon := strings.LastIndexByte(target, ':')
	if colon < 0 {
		return "", "", false
	}
	host, portText = target[:colon], target[colon+1:]
	if portText == "" || len(portText) > maxPort || strings.Trim(portText, "0123456789") != "" {
		return "", "", false
	}
	if host == "" || len(host) > maxHost {
		return "", "", false
	}
	if bracketed, ok := strings.CutPrefix(host, "["); ok {
		inner, closed := strings.CutSuffix(bracketed, "]")
		if !closed || inner == "" || strings.Trim(inner, "0123456789abcdefABCDEF:.") != "" {
			return "", "", false
		}
		return host, portText, true
	}
	if strings.Trim(host, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.") != "" {
		return "", "", false
	}
	return host, portText, true
}

// literal is whether a plain host is a bracketed IPv6 literal or dotted IPv4:
// exactly four dot-separated runs of digits, with no range check.
func literal(host string) bool {
	if strings.HasPrefix(host, "[") {
		return true
	}
	runs := strings.Split(host, ".")
	return len(runs) == 4 && !slices.ContainsFunc(runs, func(run string) bool {
		return run == "" || strings.Trim(run, "0123456789") != ""
	})
}

// HelloVerdict is the verdict on the peeked ClientHello. Its zero value refuses.
type HelloVerdict struct {
	Pass   bool
	Reason port.Reason
}

// JudgeHello decides whether the hello's server name is the CONNECT name. A peek
// whose callback never ran is not TLS, however that happened. Only extension
// 0xfe0d is refused as ECH.
func JudgeHello(hello port.Hello, name string) HelloVerdict {
	switch {
	case !hello.Seen:
		return HelloVerdict{Reason: port.NotTLS}
	case slices.Contains(hello.Extensions, echExtension):
		return HelloVerdict{Reason: port.TLSECH}
	case hello.ServerName == "":
		return HelloVerdict{Reason: port.TLSNoName}
	case hello.ServerName != name:
		return HelloVerdict{Reason: port.TLSName}
	}
	return HelloVerdict{Pass: true}
}

// Log numbers the records of one run and counts the limit. It does no I/O and
// is not safe for concurrent use: the adapter numbers under the lock it writes under.
type Log struct{ limit, seq, counted int }

// NewLog returns a log that ends after limit refused or upstream-failed records.
func NewLog(limit int) *Log { return &Log{limit: limit} }

// Stamp numbers the next record and gives it the schema and the time, in UTC to
// the second. A ready or log-full record keeps no other field. The limit-th
// refused or upstream-failed record reports full, and the adapter then stamps one
// log-full record and ends the run.
func (l *Log) Stamp(now time.Time, record port.Record) (stamped port.Record, full bool) {
	l.seq++
	record.Schema, record.Seq, record.Time = 1, l.seq, now.UTC().Format(time.RFC3339)
	switch record.Event {
	case port.Ready, port.LogFull:
		record = port.Record{Schema: record.Schema, Seq: record.Seq, Time: record.Time, Event: record.Event}
	case port.Refused, port.UpstreamFailed:
		l.counted++
		full = l.counted == l.limit
	}
	return record, full
}
