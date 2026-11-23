// Package domain validates a restricted worker request without host authority.
package domain

import (
	"slices"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

// Inspect validates only the selected file and workspace. Image lookup, host
// readiness and approval are deliberately absent from its dependencies.
//
// The reserved names are upper-case containerEnv names a selection reserves besides the fixed
// ones, each matched under ASCII case folding.
func Inspect(_ string, _ port.Files, _ port.Parser, _ port.References, _ ...string) port.Result {
	return *new(port.Result)
}

func child(path []string, key string) []string { return append(slices.Clone(path), key) }

func sortIssues(issues []port.Issue) {
	slices.SortFunc(issues, func(a, b port.Issue) int { return strings.Compare(port.Pointer(a.Path), port.Pointer(b.Path)) })
}

func interpolations(value port.Value, path []string) []port.Issue {
	var issues []port.Issue
	if value.Kind == port.String && strings.Contains(value.Text, "${") {
		issues = append(issues, port.Issue{Kind: port.Interpolation, Path: path})
	}
	for key, value := range value.Fields {
		issues = append(issues, interpolations(value, child(path, key))...)
	}
	for i, value := range value.Elements {
		issues = append(issues, interpolations(value, child(path, strconv.Itoa(i)))...)
	}
	return issues
}

func validName(value port.Value) bool {
	if value.Kind != port.String || value.Text == "" {
		return false
	}
	for _, r := range value.Text {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func identity(value port.Value) port.Failure {
	if value.Kind != port.String {
		return port.InvalidValue
	}
	if value.Text == "" || value.Text[0] < '1' || value.Text[0] > '9' {
		return port.Identity
	}
	for _, b := range value.Text {
		if b < '0' || b > '9' {
			return port.Identity
		}
	}
	return port.None
}

func workspace(value port.Value, files port.Files) port.Failure {
	if value.Kind != port.String {
		return port.InvalidValue
	}
	if value.Text == "/workspace" {
		return port.None
	}
	relative, ok := strings.CutPrefix(value.Text, "/workspace/")
	if !ok || strings.ContainsAny(relative, "\\\x00") {
		return port.Workspace
	}
	for _, part := range strings.Split(relative, "/") {
		if part == "" || part == "." || part == ".." {
			return port.Workspace
		}
	}
	if !files.Directory(relative) {
		return port.Workspace
	}
	return port.None
}

var (
	_ = identity
	_ = workspace
)
