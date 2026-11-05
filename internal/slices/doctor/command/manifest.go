package command

import (
	"fmt"
	"strconv"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

type manifestProblem struct {
	Rule    string `json:"rule"`
	Exit    int    `json:"exit"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
	Path    string `json:"path"`
	File    string `json:"file,omitempty"`
	Image   string `json:"image,omitempty"`
	Label   string `json:"label,omitempty"`
}

type manifestSettings struct {
	Name            *string           `json:"name"`
	Image           string            `json:"image"`
	WorkspaceFolder string            `json:"workspaceFolder"`
	ContainerEnv    map[string]string `json:"containerEnv"`
	ContainerUser   *string           `json:"containerUser"`
	RemoteUser      *string           `json:"remoteUser"`
}

type manifestMetadata struct {
	Checked bool     `json:"checked"`
	Present *bool    `json:"present"`
	Unused  []string `json:"unused"`
}

type manifestReport struct {
	Valid     bool              `json:"valid"`
	Path      string            `json:"path"`
	Effective *manifestSettings `json:"effective,omitempty"`
	Unused    []string          `json:"unused"`
	Metadata  manifestMetadata  `json:"image_metadata"`
}

// PresentManifest presents both file-only and explicitly supplied metadata
// validation through one boundary. It never prints raw rejected values.
func PresentManifest(u *cli.UI, result port.Result, withJSON bool) int {
	report := manifestView(result)
	valid := report.Valid
	problems := make([]manifestProblem, 0, len(result.Issues))
	code := 0
	for _, issue := range result.Issues {
		problems = append(problems, manifestDiagnostic(result, issue))
		code = cli.CodeUsage
	}
	if !valid {
		code = cli.CodeUsage
	}
	if withJSON {
		u.JSON(struct {
			Schema   int               `json:"schema"`
			OK       bool              `json:"ok"`
			Scope    string            `json:"scope"`
			Problems []manifestProblem `json:"problems"`
			Manifest manifestReport    `json:"manifest"`
		}{1, valid, "manifest", problems, report})
	} else if valid {
		_, _ = fmt.Fprintf(u.Stdout, "Manifest only\nmanifest: %s\n", printable(result.Path))
		if len(result.Unused) != 0 {
			_, _ = fmt.Fprintln(u.Stdout, "customizations.vscode is unused")
		}
		if result.Metadata.Checked {
			_, _ = fmt.Fprintln(u.Stdout, "Supplied devcontainer.metadata is unused; host readiness and approval were not checked")
		} else {
			_, _ = fmt.Fprintln(u.Stdout, "Image metadata, host readiness and approval were not checked")
		}
	}
	for _, problem := range problems {
		subject := problem.File
		if problem.Label != "" {
			subject = problem.Image + " label " + problem.Label
		}
		u.Line(fmt.Sprintf("%s: %s; %s", printable(subject+problem.Path), problem.Message, problem.Fix))
	}
	return code
}

func manifestView(result port.Result) manifestReport {
	valid := result.Effective != nil && len(result.Issues) == 0
	report := manifestReport{Valid: valid, Path: result.Path, Unused: append([]string{}, result.Unused...), Metadata: manifestMetadata{
		Checked: result.Metadata.Checked, Present: result.Metadata.Present, Unused: append([]string{}, result.Metadata.Unused...),
	}}
	if valid {
		s := result.Effective
		report.Effective = &manifestSettings{s.Name, s.Image, s.WorkspaceFolder, s.ContainerEnv, s.ContainerUser, s.RemoteUser}
	}
	return report
}

func printable(text string) string { quoted := strconv.Quote(text); return quoted[1 : len(quoted)-1] }

func manifestDiagnostic(result port.Result, issue port.Issue) manifestProblem {
	p := manifestProblem{Exit: cli.CodeUsage, Path: port.Pointer(issue.Path), File: result.Path}
	switch issue.Kind {
	case port.Missing:
		p.Rule, p.Message, p.Fix = "missing", "Selected manifest is absent", "create the policy-selected manifest; no fallback is used"
	case port.Path:
		p.Rule, p.Message, p.Fix = "path", "Selected manifest cannot be read safely", "use readable real directories and a regular file, without symlinks or special files"
	case port.Syntax:
		p.Rule, p.Message, p.Fix = "syntax", "Manifest syntax is invalid", "provide one JSON object; only comments and trailing commas extend JSON"
	case port.Complexity:
		p.Rule, p.Message, p.Fix = "complexity", "Manifest exceeds inspection limits", "reduce input to 65536 bytes, depth 32, 2048 nodes and 65536 aggregate JSON-pointer path bytes"
	case port.Duplicate:
		p.Rule, p.Message, p.Fix = "duplicate", "Object key is repeated", "keep each decoded key once at every depth"
	case port.Unsupported:
		p.Rule, p.Message, p.Fix = "unsupported", "Field is not supported", "remove fields outside the restricted worker profile, including lifecycle hooks"
	case port.InvalidValue:
		p.Rule, p.Message, p.Fix = "value", "Manifest value has an invalid type or form", "use the field types and literal values defined by the restricted profile"
	case port.Image:
		p.Rule, p.Message, p.Fix = "image", "Image is not a pinned reference", "provide an explicit named image with a lowercase sha256 digest of 64 hexadecimal digits"
	case port.Workspace:
		p.Rule, p.Message, p.Fix = "workspace", "Workspace is outside the permitted checkout layout", "use /workspace or an existing real checkout subdirectory without dot components or symlinks"
	case port.Identity:
		p.Rule, p.Message, p.Fix = "identity", "User is not a positive decimal UID", "use a positive decimal string without leading zeros, names or group suffixes"
	case port.Reserved:
		p.Rule, p.Message, p.Fix = "reserved", "Environment name is reserved", "remove controller, identity, trust, tool-loading and account namespace variables"
	case port.Interpolation:
		p.Rule, p.Message, p.Fix = "interpolation", "Variable substitution is not supported", "use literal effective strings without dollar-brace expressions"
	case port.MetadataFailure:
		p.Rule, p.Message, p.Fix = "metadata", "Image metadata is outside the descriptive-only profile", "supply strict JSON within inspection limits containing objects with only valid names and unused editor customizations"
		p.File, p.Image, p.Label = "", result.Image, "devcontainer.metadata"
	default:
		p.Rule, p.Message, p.Fix = "internal", "Unknown manifest failure", "report this as a bug"
	}
	p.Rule = "manifest." + p.Rule
	return p
}
