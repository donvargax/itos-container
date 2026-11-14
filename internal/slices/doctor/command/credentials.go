package command

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/cli"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
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
	p := readiness.Problem{Rule: issue.Rule, Exit: 1, File: "itos-container.yaml"}
	if issue.Index >= 0 {
		path := "/credentials/" + strconv.Itoa(issue.Index) + "/source"
		p.Path = &path
	}
	switch issue.Rule {
	case "credential.missing":
		p.Message, p.Fix, p.Exit = "Credential source is absent", "provide the requested source at a safe host path", 3
	case "credential.unavailable":
		p.Message, p.Fix, p.Exit = "Credential metadata or exclusion evidence is unavailable", "provide complete stable source, checkout and mount metadata", 3
	case "credential.mount_id":
		p.Message, p.Fix, p.Exit = "Credential mount identity STATX_MNT_ID is unavailable", "use a host kernel that exposes STATX_MNT_ID for every source and protected boundary", 3
	case "credential.filesystem":
		p.Message, p.Fix, p.Exit = "Overlay or FUSE backing paths cannot be excluded from credential exposure, nor can paths on NFS", "use source, checkout and state paths on filesystems with projectable backing paths", 3
	case "credential.symlink":
		p.Message, p.Fix = "Credential path contains a symbolic link", "use a source with no symbolic link in any path component"
	case "credential.ancestor":
		p.Message, p.Fix = "Credential ancestor is unsafe", "use root- or host-owned ancestors without group or other write permissions"
	case "credential.location":
		p.Message, p.Fix = "Credential source is exposed in a protected tree", "use an independent source outside checkout and protected state"
	case "credential.regular":
		p.Message, p.Fix = "Credential source is not a regular file", "provide a private regular file"
	case "credential.owner":
		p.Message, p.Fix = "Credential source belongs to another user", "use a file owned by the effective host user"
	case "credential.permissions":
		p.Message, p.Fix = "Credential source permissions expose it to another user", "remove every group and other permission bit"
	case "credential.links":
		p.Message, p.Fix = "Credential source has another filesystem link", "provide an independent file with exactly one link"
	case "credential.size":
		p.Message, p.Fix = "Credential source size is outside the supported bounds", "provide a file of 1 through 65536 bytes"
	case "credential.duplicate":
		p.Message, p.Fix = "Credential source repeats an earlier inode", "use independent files for each credential kind"
	case "state.location", "state.path", "state.unsafe", "state.unavailable":
		return approval.Problems(approval.View{Inspection: readiness.Result{Ready: true}, Failure: approval.Failure(issue.Rule)})[0]
	case "checkout.unavailable":
		return readiness.Diagnostic(readiness.Issue{Kind: readiness.CheckoutUnavailable, Subject: issue.Subject})
	case "checkout.path":
		return readiness.Diagnostic(readiness.Issue{Kind: readiness.CheckoutPath, Subject: issue.Subject})
	case "checkout.root":
		return readiness.Diagnostic(readiness.Issue{Kind: readiness.CheckoutRoot, Subject: issue.Subject})
	case "checkout.changed":
		return readiness.Diagnostic(readiness.Issue{Kind: readiness.CheckoutChanged, Subject: issue.Subject})
	case "host.unsupported":
		return readiness.Diagnostic(readiness.Issue{Kind: readiness.HostUnsupported})
	default:
		p.Message, p.Fix, p.Exit = "Unknown credential inspection failure", "report this as a bug", 70
	}
	return p
}
