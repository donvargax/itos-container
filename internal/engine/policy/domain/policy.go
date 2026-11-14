// Package domain validates the closed policy versions and supplies their defaults.
package domain

import (
	"strings"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

// Inspect checks representation first, then the supported version, then all
// independent fields. A rejected version cannot select another schema. Names
// canonicalises the host names of a version 2 allowlist, and only those.
func Inspect(source port.Source, names port.Names) port.Result {
	root, issues := source.Read()
	if len(issues) != 0 {
		return port.Result{Issues: issues}
	}
	if root.Kind != port.Object {
		return refused(port.Type)
	}
	version, exists := root.Fields["version"]
	if !exists {
		return refused(port.Required, "version")
	}
	if version.Kind != port.Number || strings.HasPrefix(version.Number.Exponent, "-") {
		return refused(port.Type, "version")
	}
	switch version.Number {
	case port.Decimal{Coefficient: "1", Exponent: "0"}:
		return inspectFields(root, 1, names)
	case port.Decimal{Coefficient: "2", Exponent: "0"}:
		return inspectFields(root, 2, names)
	case port.Decimal{Coefficient: "3", Exponent: "0"}:
		return inspectFields(root, 3, names)
	}
	return refused(port.Version, "version")
}

// inspectFields projects a root whose supported version has been read. Every
// issue records that version, so its fix names the schema the file was judged by.
func inspectFields(root port.Value, version int, names port.Names) port.Result {
	policy := port.Policy{Version: version, Network: port.Network{Mode: "offline"}}
	keys := []string{"version", "manifest", "network"}
	if version == 3 {
		keys = append(keys, "credentials")
	}
	issues := unknown(root, nil, version, keys...)
	manifest, exists := root.Fields["manifest"]
	switch {
	case !exists:
		issues = append(issues, port.Issue{Kind: port.Required, Path: []string{"manifest"}, Version: version})
	case manifest.Kind != port.String:
		issues = append(issues, port.Issue{Kind: port.Type, Path: []string{"manifest"}, Version: version})
	case !validPath(manifest.Text):
		issues = append(issues, port.Issue{Kind: port.Path, Path: []string{"manifest"}, Version: version})
	default:
		policy.Manifest = manifest.Text
	}
	if network, exists := root.Fields["network"]; exists {
		var found []port.Issue
		if version == 1 {
			found = inspectOfflineNetwork(network)
		} else {
			policy.Network, found = inspectNetwork(network, names)
			for i := range found {
				found[i].Version = version
			}
		}
		issues = append(issues, found...)
	}
	if credentials, exists := root.Fields["credentials"]; version == 3 && exists {
		var found []port.Issue
		policy.Credentials, found = inspectCredentials(credentials)
		issues = append(issues, found...)
	}
	if len(issues) != 0 {
		return port.Result{Issues: issues}
	}
	return port.Result{Policy: &policy}
}

func refused(kind port.Failure, path ...string) port.Result {
	return port.Result{Issues: []port.Issue{{Kind: kind, Path: path}}}
}

func unknown(value port.Value, path []string, version int, allowed ...string) []port.Issue {
	var issues []port.Issue
	for key := range value.Fields {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found {
			issues = append(issues, port.Issue{Kind: port.UnknownKey, Path: append(append([]string{}, path...), key), Version: version})
		}
	}
	return issues
}

// inspectOfflineNetwork is version 1's network: a closed object whose only mode is offline.
func inspectOfflineNetwork(value port.Value) []port.Issue {
	if value.Kind != port.Object {
		return []port.Issue{{Kind: port.Type, Path: []string{"network"}, Version: 1}}
	}
	issues := unknown(value, []string{"network"}, 1, "mode")
	if mode, exists := value.Fields["mode"]; exists {
		switch {
		case mode.Kind != port.String:
			issues = append(issues, port.Issue{Kind: port.Type, Path: []string{"network", "mode"}, Version: 1})
		case mode.Text != "offline":
			issues = append(issues, port.Issue{Kind: port.NetworkMode, Path: []string{"network", "mode"}, Version: 1})
		}
	}
	return issues
}

func validPath(path string) bool {
	if strings.Contains(path, "\\") {
		return false
	}
	for _, c := range path {
		if c < 32 || c == 127 {
			return false
		}
	}
	if len(path) >= 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) {
		return false
	}
	for _, component := range strings.Split(path, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}
