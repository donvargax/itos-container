package command

import (
	"fmt"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type checkoutReport struct {
	Root      string `json:"root"`
	Device    string `json:"device"`
	Inode     string `json:"inode"`
	GitDevice string `json:"git_device"`
	GitInode  string `json:"git_inode"`
}
type engineReport struct {
	Endpoint     string `json:"endpoint"`
	ID           string `json:"id"`
	APIVersion   string `json:"api_version"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
type imageReport struct {
	Reference    string `json:"reference"`
	ID           string `json:"id"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
type inspectionReport struct {
	Ready    bool            `json:"ready"`
	Checkout *checkoutReport `json:"checkout,omitempty"`
	Engine   *engineReport   `json:"engine,omitempty"`
	Image    *imageReport    `json:"image,omitempty"`
}
type readinessProblem = port.Problem

func presentReadiness(u *cli.UI, result port.Result, withJSON bool) int {
	return present(u, result, withJSON, nil, nil)
}

func presentApprovalReadiness(u *cli.UI, view approval.View, presentation approval.Presentation, withJSON bool) int {
	report := presentation.Report(view)
	var problems []approval.Problem
	if view.Failure != "" {
		problems = presentation.Problems(view)
	}
	return present(u, view.Inspection, withJSON, &report, problems)
}

func present(u *cli.UI, result port.Result, withJSON bool, approvalReport *approval.Report, approvalProblems []approval.Problem) int {
	inspection := inspectionReport{Ready: result.Ready}
	if c := result.Checkout; c != nil {
		inspection.Checkout = &checkoutReport{c.Root, c.Device, c.Inode, c.GitDevice, c.GitInode}
	}
	if e := result.Engine; e != nil {
		inspection.Engine = &engineReport{e.Endpoint, e.ID, e.APIVersion, e.OS, e.Architecture}
	}
	if i := result.Image; i != nil {
		inspection.Image = &imageReport{i.Reference, i.ID, i.OS, i.Architecture}
	}
	problems := []any{}
	code := 0
	scope := ""
	if result.Policy != nil {
		for _, issue := range result.Policy.Issues {
			scope = "policy"
			p := policyDiagnostic(issue)
			problems = append(problems, p)
			code = max(code, p.Exit)
			u.Line(printable(p.File+p.Path) + ": " + p.Message + "; " + p.Fix)
		}
	}
	var manifest *manifestReport
	if result.Manifest != nil {
		view := manifestView(*result.Manifest)
		manifest = &view
		for _, issue := range result.Manifest.Issues {
			scope = "manifest"
			p := manifestDiagnostic(*result.Manifest, issue)
			problems = append(problems, p)
			code = max(code, p.Exit)
			u.Line(printable(p.File+p.Path) + ": " + p.Message + "; " + p.Fix)
		}
	}
	for _, issue := range result.Issues {
		p := readinessDiagnostic(issue)
		problems = append(problems, p)
		code = max(code, p.Exit)
		u.Line(p.Message + "; " + p.Fix)
	}
	for _, p := range approvalProblems {
		problems = append(problems, p)
		code = max(code, p.Exit)
		u.Line(p.Rule + ": " + p.Message + "; " + p.Fix)
	}
	if approvalReport != nil && approvalReport.Status != "matching" {
		u.Line("Review with itos container approve")
	}
	if withJSON {
		u.JSON(struct {
			Approval   *approval.Report `json:"approval,omitempty"`
			Schema     int              `json:"schema"`
			Scope      string           `json:"scope,omitempty"`
			OK         bool             `json:"ok"`
			Problems   []any            `json:"problems"`
			Inspection inspectionReport `json:"inspection"`
			Manifest   *manifestReport  `json:"manifest,omitempty"`
		}{approvalReport, 1, scope, result.Ready && code == 0, problems, inspection, manifest})
	} else {
		if result.Ready {
			_, _ = fmt.Fprintln(u.Stdout, "inspection ready")
		}
		if c := inspection.Checkout; c != nil {
			_, _ = fmt.Fprintf(u.Stdout, "checkout: %s\n", printable(c.Root))
		}
		if e := inspection.Engine; e != nil {
			_, _ = fmt.Fprintf(u.Stdout, "engine: %s (%s)\n", printable(e.Endpoint), printable(e.ID))
		}
		if i := inspection.Image; i != nil {
			_, _ = fmt.Fprintf(u.Stdout, "image: %s (%s)\n", printable(i.Reference), printable(i.ID))
		}
		if approvalReport != nil {
			_, _ = fmt.Fprintf(u.Stdout, "approval %s\nfingerprint: %s\n", approvalReport.Status, approvalReport.Fingerprint)
			_, _ = fmt.Fprintln(u.Stdout, approvalReport.EffectiveText)
		}
		_, _ = fmt.Fprintln(u.Stdout, "runtime isolation is not established")
		u.Line("for scripts, use --json")
	}
	return code
}

func readinessDiagnostic(issue port.Issue) readinessProblem { return port.Diagnostic(issue) }
