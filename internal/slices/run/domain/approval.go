package domain

import "github.com/donvargax/itos-container/internal/slices/run/port"

// ApprovalRefusal preserves a safe reason without transporting request contents.
type ApprovalRefusal struct{ Reason string }

func (e ApprovalRefusal) Error() string {
	return "run approval " + e.Reason + "; use itos container approve"
}

// CheckApproval selects the exact active generation without prompting or writing.
func CheckApproval(approval port.Approval) error {
	view := approval.Inspect()
	if !view.Inspection.Ready || view.Failure != "" {
		return ApprovalRefusal{Reason: "invalid"}
	}
	if view.Status != "matching" {
		return ApprovalRefusal{Reason: view.Status}
	}
	if _, failure := approval.Freeze(view); failure != "" {
		return ApprovalRefusal{Reason: string(failure)}
	}
	return nil
}
