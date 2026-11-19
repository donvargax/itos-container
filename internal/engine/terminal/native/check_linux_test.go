package native

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

func interruptedCheck(t *testing.T) {
	p := openPair(t)
	conn, err := p.slave.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	check := func(want port.Cause) {
		t.Helper()
		l := &lease{conn: &interruptedConn{RawConn: conn}}
		if got := l.Check(); got != want {
			t.Errorf("check after interruption = %q, want %q", got, want)
		}
	}
	check("")
	if err := p.master.Close(); err != nil {
		t.Fatal(err)
	}
	check(port.Hangup)
}
