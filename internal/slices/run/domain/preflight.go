package domain

import (
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/slices/run/port"
	"strings"
)

// Prerequisite retains typed observations for the shared diagnostic classifier.
type Prerequisite struct{ View approval.View }

func (Prerequisite) Error() string { return "run prerequisite refused" }

// SnapshotRefusal never includes rejected settings or an untrusted storage path.
type SnapshotRefusal struct {
	Failure   approval.Failure
	Reference string
}

func (SnapshotRefusal) Error() string { return "run snapshot publication refused" }

// Preflight returns usable input only after atomic selection and durable storage.
func Preflight(a port.Approval, snapshots approval.Snapshots) (approval.Durable, error) {
	view := a.Inspect()
	if strings.HasPrefix(string(view.Failure), "identity.") || hasIssues(view) {
		return approval.Durable{}, Prerequisite{view}
	}
	if view.Failure != "" {
		return approval.Durable{}, ApprovalRefusal{Reason: "invalid"}
	}
	if view.Status != "matching" {
		return approval.Durable{}, ApprovalRefusal{Reason: view.Status}
	}
	if !view.Inspection.Ready {
		return approval.Durable{}, ApprovalRefusal{Reason: "invalid"}
	}
	frozen, failure := a.Freeze(view)
	if failure != "" {
		reason := string(failure)
		if strings.HasPrefix(reason, "state.") {
			reason = "invalid"
		}
		return approval.Durable{}, ApprovalRefusal{Reason: reason}
	}
	durable, failure := snapshots.Publish(frozen)
	if failure != "" {
		return approval.Durable{}, SnapshotRefusal{Failure: failure, Reference: durable.Reference}
	}
	return durable, nil
}

func hasIssues(v approval.View) bool {
	r := v.Inspection
	return len(r.Issues) > 0 || (r.Policy != nil && len(r.Policy.Issues) > 0) || (r.Manifest != nil && len(r.Manifest.Issues) > 0)
}
