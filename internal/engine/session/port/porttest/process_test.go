package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"testing"
)

func TestProcessContract(t *testing.T) {
	p := &Process{}
	ProcessContract(t, p, func() { p.Ended = true })
	for _, pid := range []int{0, -1} {
		if p, f := (&Workspace{}).HoldProcess(pid); p != nil || f != port.MountRefused {
			t.Fatal(pid, p, f)
		}
	}
	p.Failure, p.Closed = port.MountRefused, false
	if _, f := p.Exited(); f != port.MountRefused {
		t.Fatal(f)
	}
}
