package domain

import "strings"

// ApprovalRefusal preserves a safe reason without transporting request contents.
type ApprovalRefusal struct {
	Reason string
	Paths  []string
}

func (e ApprovalRefusal) Error() string {
	reason := e.Reason
	if reason == "changed" && len(e.Paths) != 0 {
		reason += " at " + strings.Join(e.Paths, ", ")
	}
	return "run approval " + reason + "; use itos container approve"
}
