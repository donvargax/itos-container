package command

import (
	"fmt"
	"strconv"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

type policyProblem = port.Problem

type effectivePolicy struct {
	Version  int              `json:"version"`
	Manifest string           `json:"manifest"`
	Network  effectiveNetwork `json:"network"`
}

type effectiveNetwork struct {
	Mode  string   `json:"mode"`
	Allow []string `json:"allow,omitempty"`
}

// presentPolicy presents policy-only success or a policy refusal that prevents
// later inspection. Neither result makes a readiness claim.
func presentPolicy(u *cli.UI, result port.Result, withJSON bool) int {
	problems := make([]policyProblem, 0, len(result.Issues))
	code := 0
	for _, issue := range result.Issues {
		problem := policyDiagnostic(issue)
		problems = append(problems, problem)
		code = max(code, problem.Exit)
	}
	var policy *effectivePolicy
	if result.Policy != nil {
		policy = &effectivePolicy{result.Policy.Version, result.Policy.Manifest, effectiveNetwork{result.Policy.Network.Mode, result.Policy.Network.Allow}}
	}
	if withJSON {
		u.JSON(struct {
			Schema   int              `json:"schema"`
			OK       bool             `json:"ok"`
			Scope    string           `json:"scope"`
			Problems []policyProblem  `json:"problems"`
			Policy   *effectivePolicy `json:"policy,omitempty"`
		}{1, code == 0, "policy", problems, policy})
	} else if policy != nil {
		manifest := strconv.Quote(policy.Manifest)
		_, _ = fmt.Fprintf(u.Stdout, "Policy only\nversion: %d\nmanifest: %s\nnetwork.mode: %s\n",
			policy.Version, manifest[1:len(manifest)-1], policy.Network.Mode)
		// Canonical names are ASCII host-name characters, so they need no escaping.
		for _, name := range policy.Network.Allow {
			_, _ = fmt.Fprintf(u.Stdout, "network.allow: %s\n", name)
		}
		_, _ = fmt.Fprintln(u.Stdout, "Host readiness and approval were not checked")
	}
	for _, problem := range problems {
		// Keys are untrusted text. Escape the terminal location separately from
		// the JSON Pointer, keeping ordinary locations free of outer quotes.
		location := strconv.Quote(problem.File + problem.Path)
		u.Line(fmt.Sprintf("%s: %s; %s", location[1:len(location)-1], problem.Message, problem.Fix))
	}
	return code
}

func policyDiagnostic(issue port.Issue) policyProblem { return port.Diagnostic(issue) }
