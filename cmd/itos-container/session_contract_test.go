package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	disk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	rundomain "github.com/donvargax/itos-container/internal/slices/run/domain"
)

// This protocol exists only in the test binary. Recovery receives a safe record
// reference and host context, never the independent observer's container ID.
type sessionContractRequest struct {
	Action, Backend, Reference, Token, Fault, Umask string
	Output, Checkpoint, Journal                     string
}

type sessionContractResult struct {
	Outcome, Reference, ID, Cleanup, PreflightFailure string
	Handoff                                           bool
	Workload                                          *int
	Events                                            []sessionContractEvent
}

type sessionContractEvent struct {
	Operation, Stage, Bytes string
	Failed                  bool
}

type sessionContractPreflight struct {
	approval.Preflight
	view ap.View
}

func (s *sessionContractPreflight) Inspect() ap.View {
	s.view = s.Preflight.Inspect()
	return s.view
}

// The red-first seam has no success oracle and performs no session operation.
// The behavior commit replaces these methods with production session calls,
// assembling its memory fakes or disk/SDK adapters here, never in feature steps.
type unavailableSessionContract struct{ operations *sessionContractIO }

func (unavailableSessionContract) create(context.Context, ap.Durable) sessionContractResult {
	return sessionContractResult{Outcome: "unavailable"}
}

func (unavailableSessionContract) cleanup(context.Context, string) sessionContractResult {
	return sessionContractResult{Outcome: "unavailable"}
}

func TestSessionContractProbe(t *testing.T) {
	path := os.Getenv("SLICE_9_PROBE_INPUT")
	if path == "" {
		t.Skip("session feature subprocess only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var request sessionContractRequest
	if err = json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	mask, err := strconv.ParseUint(request.Umask, 8, 9)
	if err != nil {
		t.Fatal(err)
	}
	restore := sessionContractUmask(int(mask))
	defer restore()
	ops := &sessionContractIO{fault: request.Fault, journal: request.Journal, stage: "intent"}
	controller := unavailableSessionContract{operations: ops}
	var result sessionContractResult
	switch request.Action {
	case "create":
		service, _ := approvalServices(os.Stdin, io.Discard, io.Discard)
		preflight := &sessionContractPreflight{Preflight: approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}}}
		snapshots := disk.Snapshots{Store: service.Store.(disk.Store), Records: canonical.JSON{}}
		handoff, preflightErr := rundomain.Preflight(preflight, snapshots)
		if preflightErr != nil {
			result.PreflightFailure = fmt.Sprintf("%T: %v (%s, %s)", preflightErr, preflightErr, preflight.view.Status, preflight.view.Failure)
			result.Outcome = "preflight-refused"
		} else {
			// The parent checkpoints independently after snapshot publication and
			// before any session work. Exit is also a bounded barrier outcome.
			writeSessionContract(t, request.Checkpoint, handoff)
			var release string
			if err := json.NewDecoder(os.Stdin).Decode(&release); err != nil || release != "continue" {
				t.Fatal("session checkpoint was not released")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			result = controller.create(ctx, handoff)
			cancel()
		}
	case "cleanup":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result = controller.cleanup(ctx, request.Reference)
		cancel()
	default:
		t.Fatal("unknown session probe action")
	}
	result.Events = append([]sessionContractEvent{}, ops.events...)
	writeSessionContract(t, request.Output, result)
}

func writeSessionContract(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	// The parent precreates the files, so even umask 0777 leaves observations
	// readable. Product files do not use this observation channel.
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
