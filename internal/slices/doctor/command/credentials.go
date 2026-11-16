package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/donvargax/itos-container/internal/cli"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type credentialGrant struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// requestedCredentials is empty when the effective root was refused before
// its policy could be read: no request is known then.
func requestedCredentials(result policy.Result) []policy.Credential {
	if result.Policy == nil {
		return nil
	}
	return result.Policy.Credentials
}

func presentCredentials(ui *cli.UI, requests []policy.Credential, result readiness.CredentialResult, json bool) int {
	grants := make([]credentialGrant, 0, len(requests))
	for _, r := range requests {
		grants = append(grants, credentialGrant{r.Kind, r.Source, "/run/itos-container/credentials/" + r.Kind})
	}
	slices.SortFunc(grants, func(a, b credentialGrant) int { return strings.Compare(a.Kind, b.Kind) })
	problems := make([]readiness.Problem, 0, len(result.Issues))
	code := 0
	for _, issue := range result.Issues {
		p := credentialProblem(issue)
		problems = append(problems, p)
		code = max(code, p.Exit)
	}
	if json {
		ui.JSON(struct {
			Schema      int                 `json:"schema"`
			Scope       string              `json:"scope"`
			OK          bool                `json:"ok"`
			Problems    []readiness.Problem `json:"problems"`
			Credentials []credentialGrant   `json:"credentials"`
		}{1, "credentials", code == 0, problems, grants})
	} else {
		_, _ = fmt.Fprintln(ui.Stdout, "Credentials only")
		for _, g := range grants {
			_, _ = fmt.Fprintf(ui.Stdout, "%s source: %s target: %s\n", g.Kind, printable(g.Source), g.Target)
		}
		_, _ = fmt.Fprintln(ui.Stdout, "Credential contents, manifest, engine and approval were not checked")
	}
	for _, p := range problems {
		location := p.Rule
		if p.Path != nil {
			location = p.File + *p.Path
		}
		ui.Line(fmt.Sprintf("%s: %s; %s", printable(location), p.Message, p.Fix))
	}
	return code
}

func credentialProblem(issue readiness.CredentialIssue) readiness.Problem {
	return readiness.CredentialDiagnostic(issue)
}
