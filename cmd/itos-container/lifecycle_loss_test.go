package main

import (
	"context"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestLifecycleEngineLossRetainsRecoverableFailure(t *testing.T) {
	m := publicMemoryFixture(t)
	m.e.WaitFailure = sp.EngineLost
	m.e.AfterWait = func() {
		// The response is lost while the primary remains alive. Reconnecting
		// fails at readiness, before cleanup can inspect or signal anything.
		for id, o := range m.e.Resources {
			o.Status, o.Running, o.PID = "running", true, 123
			m.e.Resources[id] = o
		}
		m.e.OpenFailure = sp.EnvironmentMissing
	}
	runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: m.e, Workspace: &sf.Workspace{}, Entropy: &publicMemoryEntropy{}}
	r := runner.Run(context.Background(), ap.Durable{Frozen: m.b.Frozen}, []string{"probe"}, sp.Streams{})
	if r.Outcome != sp.EngineLost || r.Exited || r.Cleanup != sp.EnvironmentMissing || len(m.e.Resources) != 1 {
		t.Fatal("transport loss invented exit or cleanup", r)
	}
	m.e.OpenFailure, m.e.WaitFailure, m.e.AfterWait = "", "", nil
	status := publicMemoryCommand(t, m.service(), "status", "", 0)
	if len(status.Sessions) != 1 {
		t.Fatal("restored status lost recovery", status)
	}
	e := status.Sessions[0]
	if e.ID != r.SessionID || e.Project != m.c.Project || e.Fingerprint != m.c.Fingerprint || e.State != "failed" || e.Resource != "running" || e.Cleanup != "pending" || e.WorkloadExit != nil || !e.StopCandidate {
		t.Fatal("restored status lost the original failed session or unknown history", e)
	}
	stopped := publicMemoryCommand(t, m.service(), "stop", r.SessionID, 0)
	if len(stopped.Sessions) != 1 || stopped.Sessions[0].Cleanup != "complete" || len(m.e.Resources) != 0 {
		t.Fatal("restored stop could not clean the original worker", stopped)
	}
}
