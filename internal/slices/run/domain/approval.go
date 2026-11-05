package domain

// ApprovalRefusal preserves a safe reason without transporting request contents.
type ApprovalRefusal struct{ Reason string }

func (e ApprovalRefusal) Error() string {
	return "run approval " + e.Reason + "; use itos container approve"
}
