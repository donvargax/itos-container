package command

import (
	"fmt"
	"strconv"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

type manifestProblem = port.Problem

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
	return port.Diagnostic(result, issue)
}
