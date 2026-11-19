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
	if strings.HasPrefix(string(v.Failure), "state.") {
		return append(problems, readiness.StateDiagnostic(string(v.Failure), v.Field))
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
		if v.RequiresTerminal() {
			p.Fix = "run itos container approve at a terminal with plain output to review and consent"
		}
	case "approve.terminal-required":
		p.Message = "Network or credential grants require the person at a terminal"
		p.Fix = "run itos container approve at a terminal with plain output to review and consent"
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

	}
	if p.Field == "" {
		p.Field = "approval"
	}
	return append(problems, p)
}
