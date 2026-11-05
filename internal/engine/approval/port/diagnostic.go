package port

import (
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"strings"
)

// Problems preserves the shared prerequisite and approval diagnostic contracts.
func Problems(v View) []Problem {
	problems := []Problem{}
	if !v.Inspection.Ready {
		problems = append(problems, readiness.Problems(v.Inspection)...)
	}
	if v.Failure == "" {
		return problems
	}
	rule := string(v.Failure)
	if !strings.HasPrefix(rule, "approve.") {
		rule = "approval." + rule
	}
	p := Problem{Rule: rule, Exit: 1, Message: "Approval was refused", Fix: "inspect with itos container doctor and run itos container approve after resolving the problem", Field: v.Field}
	switch v.Failure {
	case "identity.root":
		p.Message = "The effective host UID is root"
		p.Fix = "run as a non-root host user"
	case "identity.unavailable":
		p.Exit = 3
		p.Message = "Effective host identity is unavailable"
	case "identity.mismatch":
		p.Message = "Manifest user differs from the effective host UID"
		p.Fix = "omit the manifest user or set it to the effective host UID"
	case "approve.confirmation-required":
		p.Message = "Approval requires terminal review"
		p.Fix = "use itos container doctor, then optionally --fingerprint <value> to approve exactly those settings"
	case "approve.declined":
		p.Message = "Approval declined"
		p.Fix = "run itos container approve when ready to review and consent"
	case "approve.fingerprint":
		p.Message = "Fingerprint differs from the current effective settings"
		p.Fix = "inspect current settings with itos container doctor before approving"
	case "approve.fingerprint-format":
		p.Exit = 2
		p.Message = "Fingerprint must be sha256: followed by 64 lowercase hexadecimal digits"
	case "approve.changed":
		p.Message = "Settings or identities changed during review"
		p.Fix = "run itos container approve for a fresh review"
	case "approve.conflict":
		p.Message = "Another approval superseded the reviewed generation"
		p.Fix = "run itos container approve for a fresh review"
	case "state.location":
		p.Exit = 2
		p.Message = "Host state location must be absolute"
		p.Fix = "set an absolute XDG_STATE_HOME or an absolute HOME for its fallback"
	case "state.path":
		p.Message = "Approval state path is unsafe or was substituted"
		p.Fix = "use a stable state tree outside the checkout without symbolic links"
	case "state.unsafe":
		p.Message = "Approval state ownership, permissions or file type is unsafe"
		p.Fix = "have the host user inspect the state; managed directories must be owned mode 0700 and regular records mode 0600"
	case "state.invalid":
		p.Message = "Active approval record is invalid"
		p.Fix = "have the host user inspect the invalid state before requesting fresh approval"
	case "state.unavailable":
		p.Exit = 3
		p.Message = "Approval state is unavailable"
	case "state.write":
		p.Exit = 75
		p.Message = "Approval publication could not be confirmed durable"
		p.Fix = "inspect actual state with itos container doctor before retrying approval"
	}
	if p.Field == "" {
		p.Field = "approval"
	}
	return append(problems, p)
}
