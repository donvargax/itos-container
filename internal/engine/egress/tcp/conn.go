package tcp

import (
	"errors"
	"net"
	"os"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/domain"
	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// The answers, exactly. A refusal before the 200 gets a 403 or 502 with no body;
// a refusal after it gets no answer at all.
const (
	established = "HTTP/1.1 200 Connection Established\r\n\r\n"
	forbidden   = "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
	badGateway  = "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
)

// handle serves one client in the order of the design record: read the head,
// judge it, dial, answer 200, peek the hello, replay what was read and splice.
// Every refusal writes its record before the client is answered. When a record
// cannot be written the process is ending, and the client gets no answer.
func (r *run) handle(client *net.TCPConn) {
	defer r.wg.Done()
	defer r.release(client)
	input, ok := r.readHead(client)
	if !ok {
		return
	}
	head := r.list.Judge(input)
	if !head.Pass {
		r.answer(client, port.Refused, head.Reason, head, forbidden)
		return
	}
	upstream, event, reason := r.proxy.connect(r.ctx, head.Target)
	if upstream == nil {
		answer := forbidden
		if event == port.UpstreamFailed {
			answer = badGateway
		}
		r.answer(client, event, reason, head, answer)
		return
	}
	r.track(upstream)
	defer r.release(upstream)
	_, _ = client.Write([]byte(established))
	_ = client.SetReadDeadline(time.Now().Add(r.proxy.s.helloWait))
	sh := newShim(client, input[head.Tunnel:])
	if verdict := domain.JudgeHello(peek(sh), head.Target); !verdict.Pass {
		r.log.write(record(port.Refused, verdict.Reason, head))
		return
	}
	_ = client.SetReadDeadline(time.Time{})
	if _, err := upstream.Write(append(sh.recorded, sh.unserved()...)); err != nil {
		r.log.write(record(port.UpstreamFailed, port.Connect, head))
		return
	}
	splice(client, upstream)
}

// readHead reads up to the first CRLF CRLF, over as many reads as it takes and at
// most HeadLimit bytes in all, under one deadline set at accept. It reports false
// when the client asked for nothing: a read error with no byte read that is not
// the deadline, whether the client closed or reset.
func (r *run) readHead(client *net.TCPConn) ([]byte, bool) {
	_ = client.SetReadDeadline(time.Now().Add(r.proxy.s.headWait))
	input := make([]byte, 0, domain.HeadLimit)
	for {
		n, err := client.Read(input[len(input):cap(input)])
		input = input[:len(input)+n]
		if _, found := domain.HeadEnd(input); found || len(input) == cap(input) {
			return input, true
		}
		if err != nil {
			return input, len(input) > 0 || errors.Is(err, os.ErrDeadlineExceeded)
		}
	}
}

// answer writes the record for a connection refused or failed before the 200 and
// then its answer, in that order.
func (r *run) answer(client *net.TCPConn, event port.Event, reason port.Reason, head domain.Head, answer string) {
	if r.log.write(record(event, reason, head)) {
		_, _ = client.Write([]byte(answer))
	}
}

// record is the log entry for a connection whose head gave head.
func record(event port.Event, reason port.Reason, head domain.Head) port.Record {
	return port.Record{Event: event, Reason: reason, Method: head.Method, Target: head.Target, Port: head.Port}
}
