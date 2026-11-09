// Package port defines the egress proxy's contract values: what the handler runs,
// where the refusal log goes, how a run fails and what one log record holds. It
// holds no I/O, so the domain and the fakes can share it.
package port

import "context"

// Proxy is the allowlisting CONNECT proxy. Serve validates the names, listens,
// writes the ready record and then serves until ctx ends, which is nil, or until
// it fails with one of the Failure values.
type Proxy interface {
	Serve(ctx context.Context, names []string) error
}

// Log receives the refusal log, one record to a write. *os.File satisfies it.
type Log interface {
	Write(line []byte) (int, error)
}

// Failure is why Serve ended, as a closed set the handler may print: its values
// hold no Go error text, so they leak no address, path or client detail.
type Failure string

const (
	ListInvalid  Failure = "egress allowlist is invalid"
	ListenFailed Failure = "egress proxy could not listen"
	AcceptFailed Failure = "egress proxy stopped accepting connections"
	LogFailed    Failure = "egress proxy could not write its refusal log"
	LogExhausted Failure = "egress proxy refusal log is full"
)

// Error returns the failure's text.
func (f Failure) Error() string { return "" }

// Event is the kind of one log record.
type Event string

const (
	Ready          Event = "ready"
	Refused        Event = "refused"
	UpstreamFailed Event = "upstream-failed"
	LogFull        Event = "log-full"
)

// Reason is why a connection was refused or its upstream failed.
type Reason string

const (
	NotListed    Reason = "not-listed"
	LiteralIP    Reason = "literal-ip"
	Port         Reason = "port"
	NotCanonical Reason = "not-canonical"
	NotConnect   Reason = "not-connect"
	Malformed    Reason = "malformed"
	Address      Reason = "address"
	NotTLS       Reason = "not-tls"
	TLSName      Reason = "tls-name"
	TLSNoName    Reason = "tls-no-name"
	TLSECH       Reason = "tls-ech"
	DNS          Reason = "dns"
	Connect      Reason = "connect"
)

// Record is one line of the refusal log, in the order its fields are written.
// An empty field is left out. Port is text, so a refused 0443 keeps its zero.
type Record struct {
	Schema int    `json:"schema"`
	Seq    int    `json:"seq"`
	Time   string `json:"time"`
	Event  Event  `json:"event"`
	Method string `json:"method,omitempty"`
	Target string `json:"target,omitempty"`
	Port   string `json:"port,omitempty"`
	Reason Reason `json:"reason,omitempty"`
}

// Hello is what the ClientHello peek captured. Seen is false when the peek's
// callback never ran, whatever the reason.
type Hello struct {
	Seen       bool
	ServerName string
	Extensions []uint16
}
