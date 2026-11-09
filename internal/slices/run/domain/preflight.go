package domain

import (
	"context"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
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
	return PreflightContext(context.Background(), a, snapshots)
}

func PreflightContext(ctx context.Context, a port.Approval, snapshots approval.Snapshots) (approval.Durable, error) {
	var view approval.View
	if contextual, ok := a.(approval.ContextPreflight); ok {
		view = contextual.InspectContext(ctx)
	} else {
		var err error
		view, err = budget.Call(ctx, "preflight engine inspection", func(context.Context) approval.View { return a.Inspect() })
		if err != nil {
			return approval.Durable{}, err
		}
	}
	if ctx.Err() != nil {
		return approval.Durable{}, context.Cause(ctx)
	}
	if strings.HasPrefix(string(view.Failure), "identity.") || unresolvedByApproval(view.Failure) || hasIssues(view) {
		return approval.Durable{}, Prerequisite{view}
	}
	if view.Failure == "state.busy" {
		return approval.Durable{}, budget.Failure{Operation: "preflight approval state read"}
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
	var frozen approval.Frozen
	var failure approval.Failure
	if contextual, ok := a.(approval.ContextPreflight); ok {
		frozen, failure = contextual.FreezeContext(ctx, view)
	} else {
		var err error
		frozen, failure, err = budget.Call2(ctx, "approval selection", func(context.Context) (approval.Frozen, approval.Failure) { return a.Freeze(view) })
		if err != nil {
			return approval.Durable{}, err
		}
	}
	if ctx.Err() != nil {
		return approval.Durable{}, context.Cause(ctx)
	}
	if failure == "state.busy" {
		return approval.Durable{}, budget.Failure{Operation: "approval selection"}
	}
	if unresolvedByApproval(failure) {
		view.Failure = failure
		return approval.Durable{}, Prerequisite{view}
	}
	if failure != "" {
		reason := string(failure)
		if strings.HasPrefix(reason, "state.") {
			reason = "invalid"
		}
		return approval.Durable{}, ApprovalRefusal{Reason: reason}
	}
	durable, failure, err := budget.Call2(ctx, "frozen snapshot publication", func(context.Context) (approval.Durable, approval.Failure) { return snapshots.Publish(frozen) })
	if err != nil {
		return durable, err
	}
	if failure != "" {
		return approval.Durable{}, SnapshotRefusal{Failure: failure, Reference: durable.Reference}
	}
	return durable, nil
}

// unresolvedByApproval names the state refusals that fresh consent cannot fix: the
// person must choose another location or finish recovery. Run presents the shared
// diagnostic for them instead of the generic invalid-approval request to approve.
func unresolvedByApproval(f approval.Failure) bool { return f == "state.path" || f == "state.exposed" }

func hasIssues(v approval.View) bool {
	r := v.Inspection
	return len(r.Issues) > 0 || (r.Policy != nil && len(r.Policy.Issues) > 0) || (r.Manifest != nil && len(r.Manifest.Issues) > 0)
}
