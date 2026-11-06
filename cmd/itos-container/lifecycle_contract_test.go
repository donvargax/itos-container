package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

// Executed by the black-box scenario process while the scenarios remain @wip.
// Both backends use production command/domain code. The second also exercises
// the protected record codec, binding and transaction adapter over memory files.
func TestLifecycleContract(t *testing.T) {
	names := []string{"drain0", "drain17", "signal0", "signal17", "exit-signal-race17", "grace"}
	if selected := os.Getenv("SLICE_12_CONTRACT"); selected != "" {
		names = []string{selected}
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			for _, backend := range []string{"ports", "protected records"} {
				t.Run(backend, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) { lifecycleContract(t, name, backend) })
				})
			}
		})
	}

}

func lifecycleContract(t *testing.T, name, backend string) {
	m := publicMemoryFixture(t)
	var store sp.Store = m.registry.Store
	var registry sp.Registry = m.registry
	if backend == "ports" {
		memory := &sf.Store{Codec: m.registry.Store.Codec, UID: "1000", GID: "1000", Root: m.c.Root,
			Snapshots: map[string][]byte{"snapshots/" + m.c.Project + "/1.json": m.storage.Directories["snapshots/"+m.c.Project].Files["1.json"]}}
		store = memory
		registry = &sf.Registry{Store: memory, Project: m.c.Project}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	m.e.WaitFailure = ""
	child := 0
	if strings.HasSuffix(name, "17") {
		child = 17
	}
	m.e.Exit = child
	var signalled time.Time
	var killAt time.Duration
	switch {
	case strings.HasPrefix(name, "drain"):
		m.e.DrainUntilCancel = true
		m.e.Output = []byte("available trailing bytes")
	case strings.HasPrefix(name, "signal"):
		m.e.WaitUntilCancel = true
		m.e.AfterStart = func() { signalled = time.Now(); cancel(sp.Interrupt{Signal: "SIGTERM"}) }
	case name == "exit-signal-race17":
		m.e.AfterWait = func() { cancel(sp.Interrupt{Signal: "SIGTERM"}) }
	case name == "grace":
		m.e.WaitUntilCancel, m.e.IgnoreSignal = true, true
		m.e.Exit = 137
		m.e.AfterStart = func() { signalled = time.Now(); cancel(sp.Interrupt{Signal: "SIGTERM"}) }
		m.e.After = func(event string) {
			if event == "SIGTERM" {
				time.Sleep(6 * time.Second)
			}
			if event == "SIGKILL" {
				killAt = time.Since(signalled)
			}
		}
	default:
		t.Fatal("unknown lifecycle contract", name)
	}
	runner := domain.Runner{Store: store, Registry: registry, Engine: m.e, Workspace: &sf.Workspace{}, Entropy: &publicMemoryEntropy{}}
	approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
	var out, errout bytes.Buffer
	start := time.Now()
	code := (runcommand.CLI{Context: ctx, Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}).Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
	elapsed := time.Since(start)
	if strings.HasPrefix(name, "drain") {
		want := child
		if want == 0 {
			want = 75
		}
		if code != want || !strings.Contains(errout.String(), "run.stream-timeout") || elapsed != 10*time.Second {
			t.Fatalf("stalled drain: exit %d, elapsed %s, diagnostic %s", code, elapsed, errout.String())
		}
		if out.String() != "available trailing bytes" {
			t.Fatal("available final bytes lost", out.String())
		}
	} else {
		want := child
		if name == "grace" {
			want = 137
		}
		if code != want || !strings.Contains(errout.String(), "run.cancelled") || !strings.Contains(errout.String(), "SIGTERM") {
			t.Fatalf("cancellation lost observed exit or first signal: %d %s", code, errout.String())
		}
		if name == "grace" && killAt != 10*time.Second {
			t.Fatal("grace was restarted after delivery", killAt)
		}
	}
	r := (domain.Recovery{Registry: registry, Engine: m.e}).Status(context.Background())
	if len(r.Failures) != 0 || len(r.Entries) != 1 || r.Entries[0].Cleanup != "complete" || r.Entries[0].WorkloadExit == nil || *r.Entries[0].WorkloadExit != m.e.Exit || len(m.e.Resources) != 0 {
		t.Fatal("owned cleanup or authoritative exit lost", r, m.e.Events)
	}
}
