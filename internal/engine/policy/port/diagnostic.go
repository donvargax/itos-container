package port

import (
	"strconv"
	"strings"
)

// Problem is the stable public diagnostic contract for this inspection.
type Problem struct {
	Rule    string `json:"rule"`
	Exit    int    `json:"exit"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
	File    string `json:"file"`
	Path    string `json:"path"`
}

// Diagnostic maps a typed issue without exposing rejected source values.
func Diagnostic(issue Issue) Problem {
	p := Problem{Exit: 2, File: "itos-container.yaml"}
	for _, component := range issue.Path {
		p.Path += "/" + strings.ReplaceAll(strings.ReplaceAll(component, "~", "~0"), "/", "~1")
	}
	switch issue.Kind {
	case Missing:
		p.Rule, p.Message, p.Fix = "missing", "Policy file is absent", "create itos-container.yaml with an explicit version and manifest"
	case Unreadable:
		p.Rule, p.Message, p.Fix = "unreadable", "Policy file cannot be read", "provide a readable regular policy file of at most 65536 bytes, not a symbolic link"
		p.Exit = 3
	case Empty:
		p.Rule, p.Message, p.Fix = "empty", "Policy contains no data", "provide a version and manifest"
	case Syntax:
		p.Rule, p.Message, p.Fix = "syntax", "Policy syntax is invalid", "use one UTF-8 YAML document with valid core scalar values and no directive lines"
	case MultipleDocuments:
		p.Rule, p.Message, p.Fix = "multiple_documents", "Policy has more than one document", "keep exactly one YAML document"
	case Complexity:
		p.Rule, p.Message, p.Fix = "complexity", "Policy representation is too complex", "reduce the policy to at most 32 levels, 2048 nodes and 65536 aggregate path bytes"
	case DuplicateKey:
		p.Rule, p.Message, p.Fix = "duplicate_key", "Mapping key is repeated", "keep each decoded key only once"
	case NonStringKey:
		p.Rule, p.Message, p.Fix = "non_string_key", "Mapping key is not a string", "use string keys"
	case Anchor:
		p.Rule, p.Message, p.Fix = "anchor", "Anchor is not permitted", "write literal data without anchors"
	case Alias:
		p.Rule, p.Message, p.Fix = "alias", "Alias is not permitted", "write literal data without aliases"
	case MergeKey:
		p.Rule, p.Message, p.Fix = "merge_key", "Merge key is not permitted", "write fields explicitly without merge keys"
	case Tag:
		p.Rule, p.Message, p.Fix = "tag", "Tag is not a permitted data type", "use only JSON-compatible core types"
	case NonFinite:
		p.Rule, p.Message, p.Fix = "non_json_number", "Number is not finite", "use a finite number"
	case Required:
		p.Rule, p.Message, p.Fix = "required", "Required field is absent", "provide the required field"
	case Type:
		p.Rule, p.Message, p.Fix = "type", "Value has the wrong type", "use the field type defined by policy version "+versions(issue.Version)
	case Version:
		p.Rule, p.Message, p.Fix = "version", "Policy version is unsupported", "use policy version "+versions(issue.Version)
		if issue.Version != 0 {
			// A valid file some commands cannot act on yet: they can still show what it asks for.
			p.Message = "Policy version is not accepted by this command yet"
			p.Fix = "use policy version 1, or read what a version " + versions(issue.Version) + " file requests with doctor --policy-only"
			if issue.Version == 3 {
				p.Fix += " or doctor --credentials-only"
			}
		}
	case UnknownKey:
		p.Rule, p.Message, p.Fix = "unknown_key", "Field is not supported", "remove fields absent from the version "+versions(issue.Version)+" schema"
	case NetworkMode:
		p.Rule, p.Message, p.Fix = "network_mode", "Network mode is unsupported", "use offline mode"
		if issue.Version >= 2 {
			p.Fix = "use offline or allowlist mode"
		}
	case Path:
		p.Rule, p.Message, p.Fix = "path", "Manifest path is invalid", "use a relative slash-separated path without empty or dot components, drives, backslashes or controls"
	case HostName:
		p.Rule, p.Message, p.Fix = "host_name", "Host name is not a full DNS name", "use a full DNS host name such as api.example.com, without a wildcard, address, port, URL or space"
	case HostDuplicate:
		p.Rule, p.Message, p.Fix = "host_duplicate", "Host name repeats an earlier entry", "list each host name once"
	case AllowEmpty:
		p.Rule, p.Message, p.Fix = "allow_empty", "Allowlist is empty", "list at least one host name, or use offline mode"
	case AllowOffline:
		p.Rule, p.Message, p.Fix = "allow_offline", "Allowlist needs allowlist mode", "set network.mode to allowlist, or remove allow"
	case CredentialsEmpty:
		p.Rule, p.Message, p.Fix = "credentials_empty", "Credential requests are empty", "request at least one source, or omit credentials"
	case CredentialKind:
		p.Rule, p.Message, p.Fix = "credential_kind", "Credential kind is unsupported", "use claude-token or github-token"
	case CredentialDuplicate:
		p.Rule, p.Message, p.Fix = "credential_duplicate", "Credential kind repeats an earlier entry", "request each credential kind once"
	case CredentialSource:
		p.Rule, p.Message, p.Fix = "credential_source", "Credential source path is invalid", "use a literal absolute path of at most 4095 UTF-8 bytes without empty or dot components, backslashes or controls"
	default:
		p.Rule, p.Message, p.Fix = "internal", "Unknown policy failure", "report this as a bug"
		p.Exit = 70
	}
	p.Rule = "policy." + p.Rule
	return p
}

// versions names the schemas a fix may point at: the one the file was read
// against, or both supported ones where it declared none.
func versions(version int) string {
	if version == 0 {
		return "1, 2 or 3"
	}
	return strconv.Itoa(version)
}
