package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	disk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	session "github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
)

// The observer delegates to the real runner and exports its actual result. It
// neither decides the outcome nor gives the command a precomputed answer.
type offlineContractRunner struct {
	run func(context.Context, ap.Durable, []string, sp.Streams) sp.RunResult
}

func (r offlineContractRunner) Run(ctx context.Context, d ap.Durable, argv []string, streams sp.Streams) sp.RunResult {
	return r.run(ctx, d, argv, streams)
}
func offlineContract(t *testing.T, request sessionContractRequest, controllerFor func() session.Service) sessionContractResult {
	t.Helper()
	var result sessionContractResult
	var memory *fake.RunEngine
	assemble := func() session.Runner {
		service := controllerFor()
		runner := session.Runner{Store: service.Store, Entropy: service.Entropy, Workspace: &sessiondisk.Workspace{}}
		if request.Backend == "memory" {
			memory = &fake.RunEngine{Engine: service.Engine.(*fake.Engine)}
			if request.Boundary != "" {
				memory.Boundary = sp.IsolationUnverified
				if request.Boundary == "missing security configuration" || request.Boundary == "unavailable boundary inspection" {
					memory.Boundary = sp.IsolationInspection
				}
			}
			runner.Engine, runner.Workspace = memory, &fake.Workspace{}
		} else {
			runner.Engine = service.Engine.(sp.RunEngine)
		}
		return runner
	}
	if request.Action == "run-cleanup" {
		result = sessionContractOutput(assemble().Cleanup(context.Background(), request.Reference))
	} else {
		service, _ := approvalServices(os.Stdin, io.Discard, io.Discard)
		preflight := &sessionContractPreflight{Preflight: approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}}}
		observer := offlineContractRunner{run: func(ctx context.Context, frozen ap.Durable, argv []string, streams sp.Streams) sp.RunResult {
			writeSessionContract(t, request.Checkpoint, frozen)
			var release string
			if err := json.NewDecoder(os.Stdin).Decode(&release); err != nil || release != "continue" {
				t.Fatal("offline checkpoint was not released")
			}
			actual := assemble().Run(ctx, frozen, argv, streams)
			result.Outcome, result.Cleanup, result.Reference, result.ID = string(actual.Outcome), string(actual.Cleanup), actual.Reference, actual.ID
			if actual.Exited {
				result.Workload = &actual.Exit
			}
			return actual
		}}
		command := runcommand.CLI{Approval: preflight, Snapshots: disk.Snapshots{Store: service.Store.(disk.Store), Records: canonical.JSON{}}, Runner: observer, Args: []string{"--", "probe", "literal;$HOME"}}
		var out, errout bytes.Buffer
		result.Code = command.Run(&cli.UI{In: bytes.NewReader(nil), Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
		result.Diagnostic = errout.String()
		result.PreflightFailure = fmt.Sprintf("status=%s failure=%s ready=%v", preflight.view.Status, preflight.view.Failure, preflight.view.Inspection.Ready)
	}
	if memory != nil {
		result.RunEvents = memory.Events
	}
	return result
}
