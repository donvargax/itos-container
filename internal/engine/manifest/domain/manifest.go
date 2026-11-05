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
func Inspect(path string, files port.Files, parser port.Parser, references port.References) port.Result {
	result := port.Result{Path: path}
	data, failure := files.Read(path)
	if failure != port.None {
		result.Issues = []port.Issue{{Kind: failure}}
		return result
	}
	value, issues := parser.Parse(data, false)
	if len(issues) != 0 {
		result.Issues = issues
		return result
	}
	if value.Kind != port.Object {
		result.Issues = []port.Issue{{Kind: port.InvalidValue}}
		return result
	}
	settings := &port.Settings{WorkspaceFolder: "/workspace", ContainerEnv: map[string]string{}}
	if _, exists := value.Fields["image"]; !exists {
		result.Issues = append(result.Issues, port.Issue{Kind: port.Image, Path: []string{"image"}})
	}
	for key, value := range value.Fields {
		path := []string{key}
		if slices.Contains([]string{"name", "image", "workspaceFolder", "containerUser", "remoteUser"}, key) || key == "containerEnv" && value.Kind != port.Object {
			if issues := interpolations(value, path); len(issues) != 0 {
				result.Issues = append(result.Issues, issues...)
				continue
			}
		}
		switch key {
		case "image":
			if value.Kind != port.String || !references.Valid(value.Text) {
				result.Issues = append(result.Issues, port.Issue{Kind: port.Image, Path: path})
			} else {
				settings.Image = value.Text
			}
		case "name":
			if !validName(value) {
				result.Issues = append(result.Issues, port.Issue{Kind: port.InvalidValue, Path: path})
			} else {
				settings.Name = &value.Text
			}
		case "workspaceFolder":
			failure := workspace(value, files)
			if failure != port.None {
				result.Issues = append(result.Issues, port.Issue{Kind: failure, Path: path})
			} else {
				settings.WorkspaceFolder = value.Text
			}
		case "containerEnv":
			settings.ContainerEnv, issues = environment(value, path)
			result.Issues = append(result.Issues, issues...)
		case "containerUser", "remoteUser":
			failure := identity(value)
			if failure != port.None {
				result.Issues = append(result.Issues, port.Issue{Kind: failure, Path: path})
			} else if key == "containerUser" {
				settings.ContainerUser = &value.Text
			} else {
				settings.RemoteUser = &value.Text
			}
		case "customizations":
			unused, issues := customizations(value, path)
			result.Unused = append(result.Unused, unused...)
			result.Issues = append(result.Issues, issues...)
		default:
			result.Issues = append(result.Issues, port.Issue{Kind: port.Unsupported, Path: path})
		}
	}
	sortIssues(result.Issues)
	slices.Sort(result.Unused)
	if len(result.Issues) == 0 {
		result.Effective, result.Image = settings, settings.Image
	}
	return result
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
