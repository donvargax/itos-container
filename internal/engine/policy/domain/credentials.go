package domain

import (
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func inspectCredentials(value port.Value) ([]port.Credential, []port.Issue) {
	issue := func(kind port.Failure, path ...string) port.Issue {
		return port.Issue{Kind: kind, Path: append([]string{"credentials"}, path...), Version: 3}
	}
	if value.Kind != port.Array {
		return nil, []port.Issue{issue(port.Type)}
	}
	if len(value.Items) == 0 {
		return nil, []port.Issue{issue(port.CredentialsEmpty)}
	}
	var grants []port.Credential
	var issues []port.Issue
	seen := map[string]bool{}
	for i, entry := range value.Items {
		index := strconv.Itoa(i)
		if entry.Kind != port.Object {
			issues = append(issues, issue(port.Type, index))
			continue
		}
		issues = append(issues, unknown(entry, []string{"credentials", index}, 3, "kind", "source")...)
		kind, exists := entry.Fields["kind"]
		switch {
		case !exists:
			issues = append(issues, issue(port.Required, index, "kind"))
		case kind.Kind != port.String:
			issues = append(issues, issue(port.Type, index, "kind"))
		case kind.Text != "claude-token" && kind.Text != "github-token":
			issues = append(issues, issue(port.CredentialKind, index, "kind"))
		case seen[kind.Text]:
			issues = append(issues, issue(port.CredentialDuplicate, index, "kind"))
		default:
			seen[kind.Text] = true
		}
		source, exists := entry.Fields["source"]
		switch {
		case !exists:
			issues = append(issues, issue(port.Required, index, "source"))
		case source.Kind != port.String:
			issues = append(issues, issue(port.Type, index, "source"))
		case !credentialPath(source.Text):
			issues = append(issues, issue(port.CredentialSource, index, "source"))
		}
		grants = append(grants, port.Credential{Kind: kind.Text, Source: source.Text})
	}
	return grants, issues
}

// A source has at most 4095 bytes, the longest path Linux names: PATH_MAX
// counts the terminator, so no file can be inspected under a longer one.
func credentialPath(path string) bool {
	if len(path) > 4095 || !strings.HasPrefix(path, "/") || strings.Contains(path, "\\") {
		return false
	}
	for _, c := range path {
		if c < 32 || c == 127 {
			return false
		}
	}
	for _, component := range strings.Split(path[1:], "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}
