package docker

import (
	"context"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	tp "github.com/donvargax/itos-container/internal/engine/terminal/port"
	"github.com/moby/moby/api/types/container"
	"testing"
)

func TestTerminalEngineContracts(t *testing.T) {
	t.Run("SDK", func(t *testing.T) {
		var api *runAPI
		st.TerminalEngineContract(t, func() (sp.RunEngine, sp.Binding) {
			e, b, a := runSDKFixture(t)
			api = a
			return e, b
		})
		api.mu.Lock()
		defer api.mu.Unlock()
		if api.resource == nil || !api.resource.Config.Tty || api.resource.HostConfig.ConsoleSize != [2]uint{37, 113} {
			t.Fatal("TTY initial dimensions not sent on create")
		}
		if len(api.resizes) != 1 || api.resizes[0] != "h=53&w=167" {
			t.Fatal("resize wire dimensions", api.resizes)
		}
	})
	t.Run("fake", func(t *testing.T) {
		st.TerminalEngineContract(t, func() (sp.RunEngine, sp.Binding) {
			b, _, e := st.Fixture()
			return &st.RunEngine{Engine: e, Exit: 7, Output: []byte("stdout\x00\xff"), ErrorOutput: []byte("stderr\x00\xfe")}, b
		})
	})
}

func TestTerminalInspectionRefusesChangedTTYAndSize(t *testing.T) {
	for _, change := range []func(*container.InspectResponse){
		func(r *container.InspectResponse) { r.Config.Tty = false },
		func(r *container.InspectResponse) { r.HostConfig.ConsoleSize = [2]uint{113, 37} },
	} {
		e, b, a := runSDKFixture(t)
		c, f := e.OpenRun(context.Background(), b)
		if f != "" {
			t.Fatal(f)
		}
		defer c.Close()
		if f := c.(sp.TerminalConnection).SetTerminal(tp.Dimensions{Columns: 113, Rows: 37}); f != "" {
			t.Fatal(f)
		}
		id, f := c.CreateRun(context.Background(), st.RunIntent(b), []string{"probe"})
		if f != "" {
			t.Fatal(f)
		}
		a.mu.Lock()
		a.defect = change
		a.mu.Unlock()
		if f := c.VerifyRun(context.Background(), id, []string{"probe"}); f != sp.IsolationUnverified {
			t.Fatal("changed terminal admitted", f)
		}
	}
}

func TestTerminalResizeRefusesChangedDaemonPin(t *testing.T) {
	e, b, a := runSDKFixture(t)
	c, failure := e.OpenRun(t.Context(), b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	e.Readiness.(*st.Readiness).Current = false
	if got := c.(sp.TerminalConnection).Resize(t.Context(), "worker", tp.Dimensions{Columns: 113, Rows: 37}); got != sp.Refused {
		t.Fatal("changed daemon pin admitted", got)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.resizes) != 0 {
		t.Fatal("resize reached a replaced daemon")
	}
}
