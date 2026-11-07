package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"
)

// The original worker stays alive independently of attach/stdin. Only a fresh
// atomically published challenge starts a round. The harness releases post-loss
// nonces after the tracer has reaped the killed controller, never in advance.
func loss(path string) {
	previous := ""
	for {
		var c config
		b, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(b, &c) != nil || c.Nonce == "" || c.Nonce == previous {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		previous = c.Nonce
		f, err := os.Create("/workspace/slice-15-report.next")
		must(err)
		reportOutput = f
		emit(report{Kind: "round", Nonce: c.Nonce, Values: ids()})
		identity(c)
		filesystem(c)
		host(c)
		for _, mode := range []*config{c.Network, c.DNS} {
			mode.Nonce = c.Nonce
			emit(attempts(*mode, "first"))
			emit(attempts(*mode, "running"))
		}
		lossLoopback(c)
		must(f.Close())
		reportOutput = os.Stdout
		must(os.Rename("/workspace/slice-15-report.next", "/workspace/slice-15-report"))
	}
}

func lossLoopback(c config) {
	// Unlike the one-shot network case, close this listener before the next
	// round. It must not turn the next host-loopback attempt into a local success.
	l, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Network.Port)))
	must(err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		b := make([]byte, 1024)
		if _, err = conn.Read(b); err == nil {
			_, _ = io.WriteString(conn, "worker:"+c.Nonce+"\n")
		}
	}()
	v, err := exchange("tcp4", l.Addr().String(), c.Nonce)
	_ = l.Close()
	<-done
	emit(report{Kind: "loopback", Nonce: c.Nonce, Values: map[string]string{"reply": v, "error": fmt.Sprint(err)}})
}
