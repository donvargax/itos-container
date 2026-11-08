package main

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	session "github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	rf "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

// Report only observations from the real command and runner. The engine fake
// supplies a classified failure, then the interrupt arrives before Run sees it.
func TestForegroundInterruptContract(t *testing.T) {
	output := os.Getenv("SLICE_10_INTERRUPT_OUTPUT")
	if output == "" {
		t.Skip("foreground interrupt feature subprocess only")
	}
	b, store, engine := sf.Fixture()
	phase := os.Getenv("SLICE_10_INTERRUPT_PHASE")
	failure := sp.Outcome(os.Getenv("SLICE_10_INTERRUPT_OUTCOME"))
	worker := &sf.RunEngine{Engine: engine}
	if phase == "launch" {
		worker.Boundary = failure
	} else {
		worker.WaitFailure = failure
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	runner := session.Runner{Store: store, Entropy: sf.Entropy{Value: strings.Repeat("1", 64)}, Workspace: sf.MountWorkspace(b),
		Engine: interruptContractEngine{worker, phase, cancel}}
	approval := &rf.Approval{View: ap.View{Status: "matching", Inspection: af.Ready(),
		Candidate: ap.Candidate{Project: b.Frozen.Project, Fingerprint: b.Frozen.Fingerprint, Canonical: b.Frozen.Settings},
		Record:    ap.Record{Generation: b.Frozen.Generation}}}
	var stdout, stderr bytes.Buffer
	command := runcommand.CLI{Context: ctx, Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}
	code := command.Run(&cli.UI{Stdout: &stdout, Stderr: &stderr, Classify: runcommand.Classify})
	writeSessionContract(t, output, struct {
		Code       int
		Diagnostic string
		Cleaned    bool
		Started    bool
	}{code, stderr.String(), len(engine.Resources) == 0 && slices.Contains(worker.Events, "remove"), slices.Contains(worker.Events, "start")})
}

type interruptContractEngine struct {
	sp.RunEngine
	phase  string
	cancel context.CancelCauseFunc
}

func (e interruptContractEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	c, failure := e.RunEngine.OpenRun(ctx, b)
	if failure != "" {
		return nil, failure
	}
	return interruptContractConnection{c, e.phase, e.cancel}, ""
}

type interruptContractConnection struct {
	sp.RunConnection
	phase  string
	cancel context.CancelCauseFunc
}

func (c interruptContractConnection) VerifyRun(ctx context.Context, id string, argv []string) sp.Outcome {
	failure := c.RunConnection.VerifyRun(ctx, id, argv)
	if c.phase == "launch" {
		c.cancel(sp.Interrupt{Signal: "SIGINT"})
	}
	return failure
}

func (c interruptContractConnection) Wait(ctx context.Context, id string) (int, sp.Outcome) {
	exit, failure := c.RunConnection.Wait(ctx, id)
	if c.phase == "wait" {
		c.cancel(sp.Interrupt{Signal: "SIGTERM"})
	}
	return exit, failure
}
