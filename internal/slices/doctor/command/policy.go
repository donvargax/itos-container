package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

type policyProblem struct {
	Rule    string `json:"rule"`
	Exit    int    `json:"exit"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
	File    string `json:"file"`
	Path    string `json:"path"`
}

type effectivePolicy struct {
	Version  int              `json:"version"`
	Manifest string           `json:"manifest"`
	Network  effectiveNetwork `json:"network"`
}

type effectiveNetwork struct {
	Mode string `json:"mode"`
}

// presentPolicy presents policy inspection explicitly as limited validation. It makes
// no readiness claim, including when invoked without the policy-only flag.
func presentPolicy(u *cli.UI, result port.Result, withJSON bool) int {
	problems := make([]policyProblem, 0, len(result.Issues))
	code := 0
	for _, issue := range result.Issues {
		problem := policyDiagnostic(issue)
		problems = append(problems, problem)
		code = max(code, problem.Exit)
	}
	var policy *effectivePolicy
	if result.Policy != nil {
		policy = &effectivePolicy{result.Policy.Version, result.Policy.Manifest, effectiveNetwork{result.Policy.Network.Mode}}
	}
	if withJSON {
		u.JSON(struct {
			Schema   int              `json:"schema"`
			OK       bool             `json:"ok"`
			Scope    string           `json:"scope"`
			Problems []policyProblem  `json:"problems"`
			Policy   *effectivePolicy `json:"policy,omitempty"`
		}{1, code == 0, "policy", problems, policy})
	} else if policy != nil {
		manifest := strconv.Quote(policy.Manifest)
		_, _ = fmt.Fprintf(u.Stdout, "Policy only\nversion: %d\nmanifest: %s\nnetwork.mode: %s\nHost readiness and approval were not checked\n",
			policy.Version, manifest[1:len(manifest)-1], policy.Network.Mode)
	}
	for _, problem := range problems {
		// Keys are untrusted text. Escape the terminal location separately from
		// the JSON Pointer, keeping ordinary locations free of outer quotes.
		location := strconv.Quote(problem.File + problem.Path)
		u.Line(fmt.Sprintf("%s: %s; %s", location[1:len(location)-1], problem.Message, problem.Fix))
	}
	return code
}

func policyDiagnostic(issue port.Issue) policyProblem {
	p := policyProblem{Exit: cli.CodeUsage, File: "itos-container.yaml"}
	for _, component := range issue.Path {
		p.Path += "/" + strings.ReplaceAll(strings.ReplaceAll(component, "~", "~0"), "/", "~1")
	}
	switch issue.Kind {
	case port.Missing:
		p.Rule, p.Message, p.Fix = "missing", "Policy file is absent", "create itos-container.yaml with an explicit version and manifest"
	case port.Unreadable:
		p.Rule, p.Message, p.Fix = "unreadable", "Policy file cannot be read", "provide a readable regular policy file of at most 65536 bytes, not a symbolic link"
		p.Exit = cli.CodeEnvironment
	case port.Empty:
		p.Rule, p.Message, p.Fix = "empty", "Policy contains no data", "provide a version and manifest"
	case port.Syntax:
		p.Rule, p.Message, p.Fix = "syntax", "Policy syntax is invalid", "use one UTF-8 YAML document with valid core scalar values"
	case port.MultipleDocuments:
		p.Rule, p.Message, p.Fix = "multiple_documents", "Policy has more than one document", "keep exactly one YAML document"
	case port.Complexity:
		p.Rule, p.Message, p.Fix = "complexity", "Policy representation is too complex", "reduce the policy to at most 32 levels and 2048 nodes, including mapping keys"
	case port.DuplicateKey:
		p.Rule, p.Message, p.Fix = "duplicate_key", "Mapping key is repeated", "keep each decoded key only once"
	case port.NonStringKey:
		p.Rule, p.Message, p.Fix = "non_string_key", "Mapping key is not a string", "use string keys"
	case port.Anchor:
		p.Rule, p.Message, p.Fix = "anchor", "Anchor is not permitted", "write literal data without anchors"
	case port.Alias:
		p.Rule, p.Message, p.Fix = "alias", "Alias is not permitted", "write literal data without aliases"
	case port.MergeKey:
		p.Rule, p.Message, p.Fix = "merge_key", "Merge key is not permitted", "write fields explicitly without merge keys"
	case port.Tag:
		p.Rule, p.Message, p.Fix = "tag", "Tag is not a permitted data type", "use only JSON-compatible core types"
	case port.NonFinite:
		p.Rule, p.Message, p.Fix = "non_json_number", "Number is not finite", "use a finite number"
	case port.Required:
		p.Rule, p.Message, p.Fix = "required", "Required field is absent", "provide the required field"
	case port.Type:
		p.Rule, p.Message, p.Fix = "type", "Value has the wrong type", "use the field type defined by policy version 1"
	case port.Version:
		p.Rule, p.Message, p.Fix = "version", "Policy version is unsupported", "use policy version 1"
	case port.UnknownKey:
		p.Rule, p.Message, p.Fix = "unknown_key", "Field is not supported", "remove fields absent from the version 1 schema"
	case port.NetworkMode:
		p.Rule, p.Message, p.Fix = "network_mode", "Network mode is unsupported", "use offline mode"
	case port.Path:
		p.Rule, p.Message, p.Fix = "path", "Manifest path is invalid", "use a relative slash-separated path without empty or dot components, drives, backslashes or controls"
	default:
		p.Rule, p.Message, p.Fix = "internal", "Unknown policy failure", "report this as a bug"
		p.Exit = cli.CodeInternal
	}
	p.Rule = "policy." + p.Rule
	return p
}
