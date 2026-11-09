// Package domain decides what the egress proxy admits: the allowlist, the CONNECT
// head, the ClientHello, and the numbering of the refusal log. It is pure: the
// adapter reads the bytes and carries out the verdicts. A verdict refuses at its
// zero value, so a forgotten branch fails closed.
package domain

import (
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// The limits of one connection and of one run.
const (
	HeadLimit  = 0
	HelloLimit = 0
	LogLimit   = 0
)

// Allowlist is a validated list of canonical names. Its zero value lists nothing.
type Allowlist struct{}

// NewAllowlist validates the list.
func NewAllowlist([]string) (Allowlist, bool) { return Allowlist{}, false }

// Head is the verdict on what the client sent first. Its zero value refuses.
type Head struct {
	Pass   bool
	Reason port.Reason
	Method string
	Target string
	Port   string
	Tunnel int
}

// HeadEnd returns the offset just after the first CRLF CRLF within the first HeadLimit bytes.
func HeadEnd([]byte) (int, bool) { return 0, false }

// Judge decides on the CONNECT head at the start of input.
func (Allowlist) Judge([]byte) Head { return Head{} }

// HelloVerdict is the verdict on the peeked ClientHello. Its zero value refuses.
type HelloVerdict struct {
	Pass   bool
	Reason port.Reason
}

// JudgeHello decides whether the hello's server name is the CONNECT name.
func JudgeHello(port.Hello, string) HelloVerdict { return HelloVerdict{} }

// Log numbers the records of one run and counts the limit.
type Log struct{}

// NewLog returns a log that ends after limit refused or upstream-failed records.
func NewLog(int) *Log { return &Log{} }

// Stamp numbers the next record.
func (*Log) Stamp(time.Time, port.Record) (port.Record, bool) { return port.Record{}, false }
