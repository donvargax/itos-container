package command

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
	rf "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

// Deliver a real interrupt when the controller next observes its parent after
// successful publication. The publication result must reach the controller
// first, so cancellation exercises the handoff to launch rather than a failed
// storage call. Err still returns the real context's cancellation state.
type publicationCancellationContext struct {
	context.Context
	interrupt context.CancelCauseFunc
	signal    string
	published atomic.Bool
}

func (c *publicationCancellationContext) Err() error {
	if c.published.Swap(false) {
		c.interrupt(session.Interrupt{Signal: c.signal})
	}
	return c.Context.Err()
}

type cancellationSnapshots struct {
	*af.Snapshots
	checkpoint *publicationCancellationContext
}

func (s cancellationSnapshots) Publish(f approval.Frozen) (approval.Durable, approval.Failure) {
	durable, failure := s.Snapshots.Publish(f)
	s.checkpoint.published.Store(true)
	return durable, failure
}

func startupApprovalFixture() *rf.Approval {
	return &rf.Approval{View: approval.View{Status: "matching", Inspection: af.Ready(), Candidate: approval.Candidate{Project: "p", Fingerprint: "hash", Canonical: "selected"}, Record: approval.Record{Generation: "1"}}}
}

func TestStartupCancellationPreventsPreflightOrLaunch(t *testing.T) {
	for _, phase := range []string{"before preflight", "after publication"} {
		for _, tc := range []struct {
			signal string
			code   int
		}{{"SIGINT", 130}, {"SIGTERM", 143}} {
			t.Run(phase+"/"+tc.signal, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					parent, cancel := context.WithCancelCause(context.Background())
					defer cancel(nil)
					ctx := &publicationCancellationContext{Context: parent, interrupt: cancel, signal: tc.signal}
					a, snapshots, runner := startupApprovalFixture(), &af.Snapshots{}, &rf.Runner{}
					c := CLI{Context: ctx, Args: []string{"--", "probe"}, Approval: a, Snapshots: snapshots, Runner: runner}
					wantPreflight := 0
					if phase == "before preflight" {
						cancel(session.Interrupt{Signal: tc.signal})
					} else {
						c.Snapshots = cancellationSnapshots{snapshots, ctx}
						wantPreflight = 1
					}
					var out, diagnostics strings.Builder
					start := time.Now()
					code := c.Run(&cli.UI{Stdout: &out, Stderr: &diagnostics, Classify: Classify})
					if code != tc.code || diagnostics.String() != "itos-container: run.cancelled "+tc.signal+"; workload outcome not-started\n" || out.Len() != 0 || time.Since(start) != 0 {
						t.Fatal("startup cancellation lost its prompt typed result", code, diagnostics.String())
					}
					if a.Inspections != wantPreflight || a.Selections != wantPreflight || len(snapshots.Values) != wantPreflight || runner.Argv != nil || runner.Frozen != (approval.Durable{}) {
						t.Fatal("cancellation entered a later phase or lost the completed snapshot")
					}
					if wantPreflight == 1 && snapshots.Values["p/1"] != (approval.Frozen{Project: "p", Generation: "1", Fingerprint: "hash", Settings: "selected"}) {
						t.Fatal("cancellation changed the published snapshot", snapshots.Values)
					}
				})
			})
		}
	}
}

type stalledStartupApproval struct{ *rf.Approval }

func (a stalledStartupApproval) InspectContext(ctx context.Context) approval.View {
	view := a.Inspect()
	<-ctx.Done()
	return view
}

func (a stalledStartupApproval) FreezeContext(_ context.Context, v approval.View) (approval.Frozen, approval.Failure) {
	return a.Freeze(v)
}

func TestStartupDeadlineDuringPreflightPreventsPublicationAndLaunch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, snapshots, runner := startupApprovalFixture(), &af.Snapshots{}, &rf.Runner{}
		c := CLI{Args: []string{"--", "probe"}, Approval: stalledStartupApproval{a}, Snapshots: snapshots, Runner: runner}
		var out, diagnostics strings.Builder
		start := time.Now()
		code := c.Run(&cli.UI{Stdout: &out, Stderr: &diagnostics, Classify: Classify})
		if code != 75 || !strings.Contains(diagnostics.String(), "run.startup-timeout") || strings.Contains(diagnostics.String(), "run.call-timeout") || out.Len() != 0 || time.Since(start) != 120*time.Second {
			t.Fatal("preflight lost the original startup deadline or classification", code, time.Since(start), diagnostics.String())
		}
		if a.Inspections != 1 || a.Selections != 0 || len(snapshots.Values) != 0 || runner.Argv != nil {
			t.Fatal("expired preflight published or launched work")
		}
	})
}
