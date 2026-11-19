package domain

import (
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

// OpenCodeAuth names the grant that carries one provider's key from a host
// OpenCode auth.json. Policy version 4 introduced it; version 3 never knew it.
const OpenCodeAuth = "opencode-auth"

func inspectCredentials(value port.Value, version int) ([]port.Credential, []port.Issue) {
	issue := func(kind port.Failure, path ...string) port.Issue {
		return port.Issue{Kind: kind, Path: append([]string{"credentials"}, path...), Version: version}
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
		kind, exists := entry.Fields["kind"]
		// The closed entry of a kind lists its own fields, so a provider
		// cannot be smuggled into a kind that selects nothing by provider.
		withProvider := version >= 4 && exists && kind.Kind == port.String && kind.Text == OpenCodeAuth
		allowed := []string{"kind", "source"}
		if withProvider {
			allowed = append(allowed, "provider")
		}
		issues = append(issues, unknown(entry, []string{"credentials", index}, version, allowed...)...)
		switch {
		case !exists:
			issues = append(issues, issue(port.Required, index, "kind"))
		case kind.Kind != port.String:
			issues = append(issues, issue(port.Type, index, "kind"))
		case !credentialKind(kind.Text, version):
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
		grant := port.Credential{Kind: kind.Text, Source: source.Text}
		if withProvider {
			provider, exists := entry.Fields["provider"]
			switch {
			case !exists:
				issues = append(issues, issue(port.Required, index, "provider"))
			case provider.Kind != port.String:
				issues = append(issues, issue(port.Type, index, "provider"))
			case !providerID(provider.Text):
				issues = append(issues, issue(port.CredentialProvider, index, "provider"))
			}
			grant.Provider = provider.Text
		}
		grants = append(grants, grant)
	}
	return grants, issues
}

func credentialKind(kind string, version int) bool {
	return kind == "claude-token" || kind == "github-token" || version >= 4 && kind == OpenCodeAuth
}

// A provider ID is one literal, case-sensitive name: ASCII letters, digits,
// dots, underscores and hyphens, at most 64 bytes, beginning with a letter or
// digit. It cannot hold a wildcard, a separator or a model name, and it is
// never inferred from the source.
func providerID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case i > 0 && (c == '.' || c == '_' || c == '-'):
		default:
			return false
		}
	}
	return true
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

// ValidCredentialPath validates the literal grammar shared by approved grants.
func ValidCredentialPath(path string) bool { return credentialPath(path) }
