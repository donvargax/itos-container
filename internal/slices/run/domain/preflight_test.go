package domain

import (
	"context"
	"errors"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/run/port/porttest"
	"testing"
	"testing/synctest"
	"time"
)

func TestPreflightNeverHandsOffAfterRefusal(t *testing.T) {
	for _, failure := range []string{"identity.root", "prerequisite", "state.unsafe", "missing", "state.path", "snapshot"} {
		a := &porttest.Approval{View: approval.View{Status: "matching", Inspection: af.Ready(), Candidate: approval.Candidate{Project: "p", Fingerprint: "hash", Canonical: "selected"}, Record: approval.Record{Generation: "1"}}}
		snapshots := &af.Snapshots{}
		switch failure {
		case "identity.root", "state.unsafe":
			a.View.Failure = approval.Failure(failure)
		case "prerequisite":
			a.View.Inspection.Ready = false
			a.View.Inspection.Issues = []readiness.Issue{{Kind: readiness.ImageMissing}}
		case "missing":
			a.View.Status = "missing"
		case "state.path":
			a.Failure = "state.path"
		case "snapshot":
			snapshots.Failure = "parent directory fsync"
		}
		h, err := Preflight(a, snapshots)
		if err == nil || err.Error() == "" || h != (approval.Durable{}) {
			t.Fatalf("%s produced usable input: %#v %v", failure, h, err)
		}
		if failure == "identity.root" || failure == "prerequisite" {
			var p Prerequisite
			if !errors.As(err, &p) {
				t.Fatal(err)
			}
		}
		if failure == "snapshot" {
			var p SnapshotRefusal
			if !errors.As(err, &p) || p.Failure != "snapshot.write" || p.Reference == "" {
				t.Fatal(err)
			}
		} else if len(snapshots.Values) != 0 {
			t.Fatal("published after refused inspection/selection")
		}
	}
}
func TestPreflightReturnsSelectedValueAndDurableReference(t *testing.T) {
	a := &porttest.Approval{View: approval.View{Status: "matching", Inspection: af.Ready(), Candidate: approval.Candidate{Project: "p", Fingerprint: "hash", Canonical: "selected"}, Record: approval.Record{Generation: "1"}}}
	snapshots := &af.Snapshots{}
	h, err := Preflight(a, snapshots)
	a.View.Candidate.Canonical = "changed"
	bytes := h.Frozen.Bytes()
	bytes[0] = 'X'
	if err != nil || h.Reference != "snapshots/p/1.json" || h.Frozen.Settings != "selected" || h.Frozen.Generation != "1" || h.Frozen.Fingerprint != "hash" || snapshots.Values["p/1"] != h.Frozen {
		t.Fatal(h, err)
	}
}

type stalledSelectionApproval struct {
	*porttest.Approval
	release <-chan struct{}
}

func (a stalledSelectionApproval) Freeze(v approval.View) (approval.Frozen, approval.Failure) {
	frozen, failure := a.Approval.Freeze(v)
	<-a.release
	return frozen, failure
}

func TestPreflightBoundsSelectionWithoutContextSupport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := &porttest.Approval{View: approval.View{Status: "matching", Inspection: af.Ready(), Candidate: approval.Candidate{Project: "p", Fingerprint: "hash", Canonical: "selected"}, Record: approval.Record{Generation: "1"}}}
		snapshots := &af.Snapshots{}
		release := make(chan struct{})
		ctx, cancel := budget.Start(context.Background(), 120*time.Second, "startup")
		defer cancel()
		start := time.Now()
		durable, err := PreflightContext(ctx, stalledSelectionApproval{a, release}, snapshots)
		elapsed := time.Since(start)
		// Finish the abandoned selection only after the controller has returned.
		close(release)
		synctest.Wait()
		var failure budget.Failure
		if !errors.As(err, &failure) || failure != (budget.Failure{Operation: "approval selection"}) || elapsed != 10*time.Second {
			t.Fatal("selection lost its finite call deadline or cause", err, elapsed)
		}
		if !errors.Is(context.Cause(ctx), err) {
			t.Fatal("selection timeout did not stop startup", context.Cause(ctx), err)
		}
		if durable != (approval.Durable{}) || a.Inspections != 1 || a.Selections != 1 || len(snapshots.Values) != 0 || len(snapshots.Events) != 0 {
			t.Fatal("late selection published a snapshot or returned usable input", durable, snapshots)
		}
	})
}

type cancellingSelectionApproval struct {
	*porttest.Approval
	cancel context.CancelCauseFunc
	cause  error
}

func (a cancellingSelectionApproval) InspectContext(context.Context) approval.View {
	return a.Inspect()
}

func (a cancellingSelectionApproval) FreezeContext(_ context.Context, v approval.View) (approval.Frozen, approval.Failure) {
	frozen, failure := a.Freeze(v)
	a.cancel(a.cause)
	return frozen, failure
}

func TestPreflightPreservesCancellationAfterSelection(t *testing.T) {
	for _, signal := range []string{"SIGINT", "SIGTERM"} {
		for _, selectionFailure := range []approval.Failure{"", "state.changed"} {
			t.Run(signal+"/"+string(selectionFailure), func(t *testing.T) {
				a := &porttest.Approval{View: approval.View{Status: "matching", Inspection: af.Ready(), Candidate: approval.Candidate{Project: "p", Fingerprint: "hash", Canonical: "selected"}, Record: approval.Record{Generation: "1"}}, Failure: selectionFailure}
				snapshots := &af.Snapshots{}
				ctx, cancel := context.WithCancelCause(context.Background())
				defer cancel(nil)
				cause := session.Interrupt{Signal: signal}
				durable, err := PreflightContext(ctx, cancellingSelectionApproval{a, cancel, cause}, snapshots)
				if !errors.Is(err, cause) {
					t.Fatal("selection replaced the original cancellation cause", err)
				}
				if durable != (approval.Durable{}) || a.Inspections != 1 || a.Selections != 1 || len(snapshots.Values) != 0 || len(snapshots.Events) != 0 {
					t.Fatal("cancelled selection published a snapshot or returned usable input", durable, snapshots)
				}
			})
		}
	}
}

func TestChangedSelectionKeepsRefusalWithoutInventingPaths(t *testing.T) {
	a := &porttest.Approval{View: approval.View{Status: "matching", Inspection: af.Ready()}, Failure: "changed"}
	snapshots := &af.Snapshots{}
	_, err := Preflight(a, snapshots)
	if err == nil || err.Error() != "run approval changed; use itos container approve" || len(snapshots.Values) != 0 {
		t.Fatal(err, snapshots)
	}
}

func TestChangedInspectionCarriesOnlyPointerDiagnostics(t *testing.T) {
	a := &porttest.Approval{View: approval.View{Status: "changed", Inspection: af.Ready(), ChangedPaths: []string{"/manifest/containerEnv", "/policy/network/allow"}}}
	snapshots := &af.Snapshots{}
	_, err := Preflight(a, snapshots)
	if err == nil || err.Error() != "run approval changed at /manifest/containerEnv, /policy/network/allow; use itos container approve" || a.Selections != 0 || len(snapshots.Values) != 0 {
		t.Fatal(err, a.Selections)
	}
	if got := (ApprovalRefusal{Reason: "missing", Paths: []string{"/ignored"}}).Error(); got != "run approval missing; use itos container approve" {
		t.Fatal(got)
	}
}
