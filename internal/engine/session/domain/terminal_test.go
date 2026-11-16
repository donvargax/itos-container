package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	tf "github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
	"testing"
)

type noTerminalConnection struct{ port.RunConnection }

type noTerminalEngine struct{ port.RunEngine }

func (e noTerminalEngine) OpenRun(ctx context.Context, b port.Binding) (port.RunConnection, port.Outcome) {
	c, f := e.RunEngine.OpenRun(ctx, b)
	return noTerminalConnection{c}, f
}

type invalidInitialTerminal struct{ terminal.Lease }

func (invalidInitialTerminal) Initial() terminal.Dimensions { return terminal.Dimensions{} }

func TestRegisteredLaunchRefusesUnsupportedTerminalBeforeCreate(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid size", true: "missing capability"}[missing], func(t *testing.T) {
			runner, registry, engine, workspace, id := registeredSessionFixture(t)
			provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
			lease, err := provider.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			selected := terminal.Lease(invalidInitialTerminal{lease})
			if missing {
				runner.Engine = noTerminalEngine{runner.Engine}
				selected = lease
			}
			c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{Terminal: selected})
			if c != nil {
				defer c.Close()
			}
			if failure != port.TerminalUnavailable || a != nil || worker != "" || len(engine.Resources) != 0 || registry.Locks[id] || !workspace.Closed {
				t.Fatalf("terminal refusal advanced launch or retained authority: %s %q", failure, worker)
			}
		})
	}
}

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
	for _, change := range []string{"healthy", "invalid size", "unavailable registry", "changed session", "stopped session", "unavailable engine", "nil engine", "foreign worker", "stopped worker"} {
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
			case "changed session":
				registry.AfterOpen = func(string) {
					r := registry.Records[id]
					r.State = "stopped"
					registry.Records[id] = r
				}
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
