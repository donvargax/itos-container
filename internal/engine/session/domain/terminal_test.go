package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	"testing"
)

type noTerminalConnection struct{ port.RunConnection }

func TestMissingTerminalCapabilityRefuses(t *testing.T) {
	c := finiteConnection{RunConnection: noTerminalConnection{}}
	if f := c.SetTerminal(terminal.Dimensions{Columns: 113, Rows: 37}); f != port.TerminalUnavailable {
		t.Fatal(f)
	}
	if f := c.Resize(t.Context(), "worker", terminal.Dimensions{Columns: 113, Rows: 37}); f != port.TerminalResize {
		t.Fatal(f)
	}
}

func TestResizeRequiresFreshOwnedRunningSession(t *testing.T) {
	for _, change := range []string{"healthy", "invalid size", "unavailable registry", "stopped session", "unavailable engine", "nil engine", "foreign worker", "stopped worker"} {
		t.Run(change, func(t *testing.T) {
			s, registry, engine, _, id := registeredSessionFixture(t)
			c, a, worker, f := s.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
			if f != "" {
				t.Fatal(f)
			}
			defer c.Close()
			defer a.Close()
			size := terminal.Dimensions{Columns: 113, Rows: 37}
			want := port.TerminalResize
			switch change {
			case "healthy":
				want = ""
			case "invalid size":
				size.Rows = 0
				want = port.TerminalUnavailable
			case "unavailable registry":
				registry.Failure = port.Incomplete
			case "stopped session":
				r := registry.Records[id]
				r.State = "stopped"
				registry.Records[id] = r
			case "unavailable engine":
				engine.OpenFailure = port.EngineLost
			case "nil engine":
				engine.OpenNil = true
			case "foreign worker":
				r := engine.Resources[worker]
				r.Name = "foreign"
				engine.Resources[worker] = r
			case "stopped worker":
				r := engine.Resources[worker]
				r.Running = false
				engine.Resources[worker] = r
			}
			if got := s.resizeOwned(context.Background(), id, worker, size); got != want {
				t.Fatalf("resize: %s want %s", got, want)
			}
		})
	}
}
