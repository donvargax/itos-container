package port

// StateDiagnostic is shared by credential readiness and protected approval state.
func StateDiagnostic(rule, field string) Problem {
	p := Problem{Rule: "approval." + rule, Exit: 1, Message: "Approval was refused", Fix: "inspect with itos container doctor and run itos container approve after resolving the problem", Field: field}
	switch rule {
	case "state.location":
		p.Exit = 2
		p.Message = "Host state location must be absolute, valid UTF-8 and at most 4095 bytes"
		p.Fix = "set an absolute XDG_STATE_HOME, or an absolute HOME for its fallback, whose selected state path including /itos-container is valid UTF-8 and at most 4095 bytes"
	case "state.exposed":
		p.Message = "Control state is exposed to an outstanding worker checkout"
		p.Fix = "choose a state location outside worker checkouts or finish recovery from the blocking worker’s original checkout; reapproval alone cannot fix exposure"
		p.Field = "state"
	case "state.path":
		p.Message = "Approval state path is unsafe or was substituted"
		p.Fix = "use a stable state tree outside the checkout without symbolic links; the account home's own path must have none either, and XDG_STATE_HOME cannot move that anchor"
	case "state.unsafe":
		p.Message = "Approval state ownership, permissions or file type is unsafe"
		p.Fix = "have the host user inspect the state; managed directories must be owned mode 0700 and regular records mode 0600"
	case "state.invalid":
		p.Message = "Active approval record is invalid"
		p.Fix = "have the host user inspect the invalid state before requesting fresh approval"
	case "state.unavailable":
		p.Exit = 3
		p.Message = "Approval state is unavailable"
		p.Fix = "resolve the cause and inspect again with itos container doctor; approve cannot clear it. The effective user needs a local /etc/passwd entry with an existing absolute home directory, so LDAP or SSSD-only accounts are refused. " +
			"A state location the catalog retains may be missing or unreadable, which refuses every selection on the account until the host user restores the original directory or removes its record under the account home's .local/state/itos-container/coordination after confirming no worker uses it. " +
			"Admission, session or intent records with no catalog beside them refuse the selection that holds them, and every selection when they are in the account home's .local/state/itos-container, so choosing another XDG_STATE_HOME does not help there; the host user moves those directories out of the tree after confirming no worker uses them"
	case "state.busy":
		p.Exit = 75
		p.Message = "Approval state lock wait exceeded the call budget"
		p.Fix = "retry after the other control-state operation completes"
	case "state.write":
		p.Exit = 75
		p.Message = "Approval publication could not be confirmed durable"
		p.Fix = "inspect actual state with itos container doctor before retrying approval"
	}
	if p.Field == "" {
		p.Field = "approval"
	}
	return p
}
