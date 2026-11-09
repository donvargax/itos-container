package domain

import (
	"slices"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

// Limits on one allowlist entry. The text limit comes first, before any
// canonicalising: punycode encoding is quadratic in the name, so a hostile 64 KiB
// name would cost seconds. A character that maps to one ASCII character takes at
// most 4 bytes, so every 253-byte name can be spelled within the limit.
const (
	maxEntryBytes = 1024
	maxNameBytes  = 253
	maxLabelBytes = 63
)

// inspectNetwork is version 2's network: offline, or an allowlist of host names.
// A wrong mode, or a mode of the wrong type, is reported alone: the list means
// nothing without one, so nothing may judge it before the mode is known.
func inspectNetwork(value port.Value, names port.Names) (port.Network, []port.Issue) {
	issue := func(kind port.Failure, path ...string) port.Issue {
		return port.Issue{Kind: kind, Path: append([]string{"network"}, path...), Version: 2}
	}
	if value.Kind != port.Object {
		return port.Network{}, []port.Issue{{Kind: port.Type, Path: []string{"network"}, Version: 2}}
	}
	issues := unknown(value, []string{"network"}, 2, "mode", "allow")
	mode := port.Value{Kind: port.String, Text: "offline"}
	if found, exists := value.Fields["mode"]; exists {
		mode = found
	}
	allow, listed := value.Fields["allow"]
	switch {
	case mode.Kind != port.String:
		return port.Network{}, append(issues, issue(port.Type, "mode"))
	case mode.Text == "offline":
		if listed {
			issues = append(issues, issue(port.AllowOffline, "allow"))
		}
		return port.Network{Mode: "offline"}, issues
	case mode.Text != "allowlist":
		return port.Network{}, append(issues, issue(port.NetworkMode, "mode"))
	case !listed:
		return port.Network{}, append(issues, issue(port.Required, "allow"))
	case allow.Kind != port.Array:
		return port.Network{}, append(issues, issue(port.Type, "allow"))
	case len(allow.Items) == 0:
		return port.Network{}, append(issues, issue(port.AllowEmpty, "allow"))
	}
	hosts, found := inspectHosts(allow.Items, names)
	return port.Network{Mode: "allowlist", Allow: hosts}, append(issues, found...)
}

// inspectHosts canonicalises every entry. A name appears once, in ascending
// byte order of its canonical spelling, so a reordered list is the same request.
// Only a canonical name can repeat, so a refused entry is never a duplicate.
func inspectHosts(items []port.Value, names port.Names) ([]string, []port.Issue) {
	var hosts []string
	var issues []port.Issue
	seen := map[string]bool{}
	for i, item := range items {
		path := []string{"network", "allow", strconv.Itoa(i)}
		if item.Kind != port.String {
			issues = append(issues, port.Issue{Kind: port.Type, Path: path, Version: 2})
			continue
		}
		name, ok := canonicalHost(item.Text, names)
		switch {
		case !ok:
			issues = append(issues, port.Issue{Kind: port.HostName, Path: path, Version: 2})
		case seen[name]:
			issues = append(issues, port.Issue{Kind: port.HostDuplicate, Path: path, Version: 2})
		default:
			seen[name] = true
			hosts = append(hosts, name)
		}
	}
	slices.Sort(hosts)
	return hosts, issues
}

// canonicalHost returns the one spelling of a full DNS host name that an entry
// stands for, or false. The check is lexical: no lookup, no public-suffix list and
// no list of special names, since what a listed name may resolve to belongs to
// whatever enforces the list.
func canonicalHost(text string, names port.Names) (string, bool) {
	if len(text) > maxEntryBytes {
		return "", false
	}
	ascii, ok := names.ASCII(text)
	if !ok {
		return "", false
	}
	// Lookup accepts .example.com, example.com.. and a 64-byte label, and turns
	// xn--.example into .example, so the shape of the name is checked here. The
	// dot is removed after mapping, so an ideographic final dot counts as well.
	name := strings.TrimSuffix(ascii, ".")
	if len(name) > maxNameBytes {
		return "", false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return "", false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > maxLabelBytes || strings.Trim(label, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			return "", false
		}
	}
	if addressLike(labels[len(labels)-1]) {
		return "", false
	}
	return name, true
}

// addressLike is whether address parsers would read a final label as a number:
// all decimal digits, or 0x followed only by hexadecimal digits. Lookup passes
// 127.1, 0x7f.0.0.1 and 1.2.3.0X4, which connect to 127.0.0.1 and 1.2.3.4.
func addressLike(label string) bool {
	digits := strings.TrimPrefix(label, "0x")
	if digits != label {
		return strings.Trim(digits, "0123456789abcdef") == ""
	}
	return strings.Trim(label, "0123456789") == ""
}
