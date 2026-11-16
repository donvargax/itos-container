package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) credentialApprovalHelpSteps(sc *godog.ScenarioContext) {
	sc.Step(`^approval help says credential and network grants require terminal review and refuse every fingerprint option$`, func() error {
		return w.credentialApprovalHelpContains(
			"Network or credential grants require the person at a real terminal; --fingerprint refuses.",
			"Bare approve without terminal stdin and stdout, or with JSON, refuses.",
		)
	})
	sc.Step(`^approval help retains the offline no-credential fingerprint route and unchanged-settings no-prompt rule$`, func() error {
		return w.credentialApprovalHelpContains(
			"Optional --fingerprint <value> approves only a matching offline grant without credentials and without a terminal.",
			"Approval is remembered until settings change; an unchanged approval does not ask again.",
		)
	})
	sc.Step(`^approval help says an unsandboxed host agent can fake a terminal and approval records what not who$`, func() error {
		return w.credentialApprovalHelpContains(
			"an agent on the host can approve offline settings through --fingerprint, or any settings through a faked terminal while unsandboxed.",
			"Approval records what was approved, not who.",
		)
	})
}

// Help wrapping is presentation, not a different consent rule. Check complete
// statements after joining whitespace so COLUMNS cannot change their meaning.
func (w *world) credentialApprovalHelpContains(statements ...string) error {
	text := strings.Join(strings.Fields(w.stdout), " ")
	for _, statement := range statements {
		if !strings.Contains(text, statement) {
			return fmt.Errorf("approval help lacks consent statement %q", statement)
		}
	}
	return nil
}
