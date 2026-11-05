package domain

import (
	"errors"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/run/port/porttest"
	"testing"
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
