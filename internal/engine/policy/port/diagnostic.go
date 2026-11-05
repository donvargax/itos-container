package port

import "strings"

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
		p.Rule, p.Message, p.Fix = "type", "Value has the wrong type", "use the field type defined by policy version 1"
	case Version:
		p.Rule, p.Message, p.Fix = "version", "Policy version is unsupported", "use policy version 1"
	case UnknownKey:
		p.Rule, p.Message, p.Fix = "unknown_key", "Field is not supported", "remove fields absent from the version 1 schema"
	case NetworkMode:
		p.Rule, p.Message, p.Fix = "network_mode", "Network mode is unsupported", "use offline mode"
	case Path:
		p.Rule, p.Message, p.Fix = "path", "Manifest path is invalid", "use a relative slash-separated path without empty or dot components, drives, backslashes or controls"
	default:
		p.Rule, p.Message, p.Fix = "internal", "Unknown policy failure", "report this as a bug"
		p.Exit = 70
	}
	p.Rule = "policy." + p.Rule
	return p
}
