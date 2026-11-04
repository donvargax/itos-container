// Package domain validates the closed offline policy and supplies its defaults.
package domain

import (
	"math/big"
	"strings"

	"github.com/donvargax/itos-container/internal/policy/port"
)

// Inspect checks representation first, then the supported version, then all
// independent fields. A rejected version cannot select another schema.
func Inspect(source port.Source) port.Result {
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
	if version.Kind != port.Number || !version.Number.IsInt() {
		return refused(port.Type, "version")
	}
	if version.Number.Cmp(big.NewRat(1, 1)) != 0 {
		return refused(port.Version, "version")
	}

	policy := port.Policy{Version: 1, Network: port.Network{Mode: "offline"}}
	issues = unknown(root, nil, "version", "manifest", "network")
	manifest, exists := root.Fields["manifest"]
	switch {
	case !exists:
		issues = append(issues, port.Issue{Kind: port.Required, Path: []string{"manifest"}})
	case manifest.Kind != port.String:
		issues = append(issues, port.Issue{Kind: port.Type, Path: []string{"manifest"}})
	case !validPath(manifest.Text):
		issues = append(issues, port.Issue{Kind: port.Path, Path: []string{"manifest"}})
	default:
		policy.Manifest = manifest.Text
	}
	if network, exists := root.Fields["network"]; exists {
		issues = append(issues, inspectNetwork(network)...)
	}
	if len(issues) != 0 {
		return port.Result{Issues: issues}
	}
	return port.Result{Policy: &policy}
}

func refused(kind port.Failure, path ...string) port.Result {
	return port.Result{Issues: []port.Issue{{Kind: kind, Path: path}}}
}

func unknown(value port.Value, path []string, allowed ...string) []port.Issue {
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
			issues = append(issues, port.Issue{Kind: port.UnknownKey, Path: append(append([]string{}, path...), key)})
		}
	}
	return issues
}

func inspectNetwork(value port.Value) []port.Issue {
	if value.Kind != port.Object {
		return []port.Issue{{Kind: port.Type, Path: []string{"network"}}}
	}
	issues := unknown(value, []string{"network"}, "mode")
	if mode, exists := value.Fields["mode"]; exists {
		switch {
		case mode.Kind != port.String:
			issues = append(issues, port.Issue{Kind: port.Type, Path: []string{"network", "mode"}})
		case mode.Text != "offline":
			issues = append(issues, port.Issue{Kind: port.NetworkMode, Path: []string{"network", "mode"}})
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
