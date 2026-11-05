package port

// Problem is the stable public diagnostic contract for this inspection.
type Problem struct {
	Rule    string `json:"rule"`
	Exit    int    `json:"exit"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
	Path    string `json:"path"`
	File    string `json:"file,omitempty"`
	Image   string `json:"image,omitempty"`
	Label   string `json:"label,omitempty"`
}

// Diagnostic maps a typed issue without exposing rejected source values.
func Diagnostic(result Result, issue Issue) Problem {
	p := Problem{Exit: 2, Path: Pointer(issue.Path), File: result.Path}
	switch issue.Kind {
	case Missing:
		p.Rule, p.Message, p.Fix = "missing", "Selected manifest is absent", "create the policy-selected manifest; no fallback is used"
	case Path:
		p.Rule, p.Message, p.Fix = "path", "Selected manifest cannot be read safely", "use readable real directories and a regular file, without symlinks or special files"
	case Syntax:
		p.Rule, p.Message, p.Fix = "syntax", "Manifest syntax is invalid", "provide one JSON object; only comments and trailing commas extend JSON"
	case Complexity:
		p.Rule, p.Message, p.Fix = "complexity", "Manifest exceeds inspection limits", "reduce input to 65536 bytes, depth 32, 2048 nodes and 65536 aggregate JSON-pointer path bytes"
	case Duplicate:
		p.Rule, p.Message, p.Fix = "duplicate", "Object key is repeated", "keep each decoded key once at every depth"
	case Unsupported:
		p.Rule, p.Message, p.Fix = "unsupported", "Field is not supported", "remove fields outside the restricted worker profile, including lifecycle hooks"
	case InvalidValue:
		p.Rule, p.Message, p.Fix = "value", "Manifest value has an invalid type or form", "use the field types and literal values defined by the restricted profile"
	case Image:
		p.Rule, p.Message, p.Fix = "image", "Image is not a pinned reference", "provide an explicit named image with a lowercase sha256 digest of 64 hexadecimal digits"
	case Workspace:
		p.Rule, p.Message, p.Fix = "workspace", "Workspace is outside the permitted checkout layout", "use /workspace or an existing real checkout subdirectory without dot components or symlinks"
	case Identity:
		p.Rule, p.Message, p.Fix = "identity", "User is not a positive decimal UID", "use a positive decimal string without leading zeros, names or group suffixes"
	case Reserved:
		p.Rule, p.Message, p.Fix = "reserved", "Environment name is reserved", "remove controller, identity, trust, tool-loading and account namespace variables"
	case Interpolation:
		p.Rule, p.Message, p.Fix = "interpolation", "Variable substitution is not supported", "use literal effective strings without dollar-brace expressions"
	case MetadataFailure:
		p.Rule, p.Message, p.Fix = "metadata", "Image metadata is outside the descriptive-only profile", "supply strict JSON within inspection limits containing objects with only valid names and unused editor customizations"
		p.File, p.Image, p.Label = "", result.Image, "devcontainer.metadata"
	default:
		p.Rule, p.Message, p.Fix = "internal", "Unknown manifest failure", "report this as a bug"
	}
	p.Rule = "manifest." + p.Rule
	return p
}
