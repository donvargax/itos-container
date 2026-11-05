package domain

import (
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

func environment(value port.Value, path []string) (map[string]string, []port.Issue) {
	if value.Kind != port.Object {
		return nil, []port.Issue{{Kind: port.InvalidValue, Path: path}}
	}
	result := map[string]string{}
	var issues []port.Issue
	for name, value := range value.Fields {
		if interpolation := interpolations(value, child(path, name)); len(interpolation) != 0 {
			issues = append(issues, interpolation...)
			continue
		}
		failure := port.None
		switch {
		case reserved(name):
			failure = port.Reserved
		case !envName(name) || value.Kind != port.String || strings.ContainsRune(value.Text, 0):
			failure = port.InvalidValue
		}
		if failure != port.None {
			issues = append(issues, port.Issue{Kind: failure, Path: child(path, name)})
		} else {
			result[name] = value.Text
		}
	}
	return result, issues
}

func envName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		if c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func reserved(name string) bool {
	// ASCII folding only; non-ASCII identifiers are rejected by envName.
	bytes := []byte(name)
	for i, c := range bytes {
		if c >= 'a' && c <= 'z' {
			bytes[i] = c - ('a' - 'A')
		}
	}
	upper := string(bytes)
	for _, exact := range []string{"HOME", "USER", "LOGNAME", "PATH", "SHELL", "ENV", "BASH_ENV", "SHELLOPTS", "BASHOPTS", "TMP", "TEMP", "TMPDIR", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS"} {
		if upper == exact {
			return true
		}
	}
	for _, prefix := range []string{"ITOS_", "DOCKER_", "CONTAINER_", "XDG_", "LD_", "DYLD_", "SSH_", "GIT_", "GH_", "GITHUB_", "AWS_", "AZURE_", "GOOGLE_", "GCLOUD_", "CLAUDE_", "ANTHROPIC_", "OPENAI_", "OPENCODE_"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

func customizations(value port.Value, path []string) ([]string, []port.Issue) {
	if value.Kind != port.Object {
		return nil, []port.Issue{{Kind: port.InvalidValue, Path: path}}
	}
	var unused []string
	var issues []port.Issue
	for key, value := range value.Fields {
		at := child(path, key)
		if key != "vscode" {
			issues = append(issues, port.Issue{Kind: port.Unsupported, Path: at})
			continue
		}
		if value.Kind != port.Object {
			issues = append(issues, port.Issue{Kind: port.InvalidValue, Path: at})
			continue
		}
		unused = append(unused, port.Pointer(at))
		for key, value := range value.Fields {
			path := child(at, key)
			switch key {
			case "settings":
				if value.Kind != port.Object {
					issues = append(issues, port.Issue{Kind: port.InvalidValue, Path: path})
				}
			case "extensions":
				if value.Kind != port.Array {
					issues = append(issues, port.Issue{Kind: port.InvalidValue, Path: path})
					continue
				}
				for i, entry := range value.Elements {
					if entry.Kind != port.String || entry.Text == "" {
						issues = append(issues, port.Issue{Kind: port.InvalidValue, Path: child(path, strconv.Itoa(i))})
					}
				}
			default:
				issues = append(issues, port.Issue{Kind: port.Unsupported, Path: path})
			}
		}
	}
	return unused, issues
}
