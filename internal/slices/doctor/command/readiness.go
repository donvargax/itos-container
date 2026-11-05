package command

import (
	"fmt"
	"strings"

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
type readinessProblem struct {
	Rule    string  `json:"rule"`
	Exit    int     `json:"exit"`
	Message string  `json:"message"`
	Fix     string  `json:"fix"`
	Path    *string `json:"path,omitempty"`
	Field   string  `json:"field,omitempty"`
	Image   string  `json:"image,omitempty"`
}

func presentReadiness(u *cli.UI, result port.Result, withJSON bool) int {
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
	if withJSON {
		u.JSON(struct {
			Schema     int              `json:"schema"`
			Scope      string           `json:"scope,omitempty"`
			OK         bool             `json:"ok"`
			Problems   []any            `json:"problems"`
			Inspection inspectionReport `json:"inspection"`
			Manifest   *manifestReport  `json:"manifest,omitempty"`
		}{1, scope, result.Ready, problems, inspection, manifest})
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
		_, _ = fmt.Fprintln(u.Stdout, "approval and runtime isolation are not established")
		u.Line("for scripts, use --json")
	}
	return code
}

func readinessDiagnostic(issue port.Issue) readinessProblem {
	p := readinessProblem{Exit: 1}
	switch issue.Kind {
	case port.HostUnsupported:
		p.Rule, p.Message, p.Fix = "host.unsupported", "Host is outside the inspection target", "use Linux amd64 with native Docker"
	case port.CheckoutRoot:
		p.Rule, p.Exit, p.Message, p.Fix = "checkout.root", 2, "Checkout root is not absolute", "set ITOS_ROOT to the absolute checkout root"
	case port.CheckoutUnavailable:
		p.Rule, p.Exit, p.Message, p.Fix = "checkout.unavailable", 3, "Checkout root is unavailable", "provide a readable checkout directory"
	case port.CheckoutLayout:
		p.Rule, p.Message, p.Fix = "checkout.layout", "Checkout layout is unsupported", "use a standalone checkout with embedded HEAD, config, objects and refs and no external Git storage"
	case port.CheckoutPath:
		p.Rule, p.Message, p.Fix = "checkout.path", "Checkout inspection path is unsafe", "use real root directories and Git metadata without symbolic links"
	case port.CheckoutChanged:
		p.Rule, p.Message, p.Fix = "checkout.changed", "Checkout identity changed during inspection", "stabilize the checkout and inspect again"
	case port.EngineRemote:
		p.Rule, p.Message, p.Fix = "engine.remote", "Remote engine transports are unsupported", "select a host-owned local Unix socket"
	case port.EngineSelection:
		p.Rule, p.Exit, p.Message, p.Fix = "engine.selection", 2, "Engine selection is unsupported", "use only DOCKER_HOST=unix:///absolute/socket without context, TLS or API overrides"
	case port.EngineUntrusted:
		p.Rule, p.Message, p.Fix = "engine.untrusted", "Engine socket belongs to the checkout", "select a host-owned socket outside the checkout"
	case port.EngineUnavailable:
		p.Rule, p.Exit, p.Message, p.Fix = "engine.unavailable", 3, "Local engine socket is unavailable", "have the host operator check the socket and its permissions"
	case port.EngineIdentityMissing:
		p.Rule, p.Message, p.Fix = "engine.identity", "Engine supplied no daemon identity", "use a native Docker engine that reports its daemon ID"
	case port.EngineAPI:
		p.Rule, p.Message, p.Fix = "engine.api", "Engine and client API ranges are incompatible", "use an engine supporting the pinned client's negotiated API"
	case port.EngineUnsupported:
		p.Rule, p.Message, p.Fix = "engine.unsupported", "Engine product is outside the inspection target", "use native Docker Engine Community or Enterprise"
	case port.EnginePlatform:
		p.Rule, p.Message, p.Fix = "engine.platform", "Engine platform observations are unsupported or inconsistent", "use a native Linux amd64 Docker engine"
	case port.EngineTimeout:
		p.Rule, p.Exit, p.Message, p.Fix = "engine.timeout", 75, "Engine inspection exceeded five seconds", "restore engine responsiveness before inspecting again"
	case port.EngineInspect:
		p.Rule, p.Exit, p.Message, p.Fix = "engine.inspect", 75, "Engine inspection failed", "have the host operator check engine health before inspecting again"
	case port.EngineChanged:
		p.Rule, p.Message, p.Fix = "engine.changed", "Engine identity changed during inspection", "stabilize the selected daemon and socket before inspecting again"
	case port.ImageMissing:
		p.Rule, p.Exit, p.Message, p.Fix = "image.missing", 3, "Requested image is not cached", "ask the host operator to prepare the exact image outside doctor"
	case port.ImageInspect:
		p.Rule, p.Exit, p.Message, p.Fix = "image.inspect", 75, "Cached image inspection was unusable", "have the host operator check the exact cached image and engine health"
	case port.ImageDigest:
		p.Rule, p.Message, p.Fix = "image.digest", "Cached image does not attest the requested repository digest", "ask the host operator to prepare the exact image outside doctor"
	case port.ImagePlatform:
		p.Rule, p.Message, p.Fix = "image.platform", "Cached image platform is incompatible", "prepare the exact requested Linux amd64 image outside doctor"
	case port.ImageUser:
		p.Rule, p.Message, p.Fix = "image.user", "Image user does not match the manifest's numeric identity request", "prepare an image with a positive numeric UID matching each specified manifest user"
	default:
		p.Rule, p.Exit, p.Message, p.Fix = "internal", 70, "Unknown inspection failure", "report this as a bug"
	}
	switch {
	case strings.HasPrefix(p.Rule, "checkout."):
		p.Path = &issue.Subject
	case strings.HasPrefix(p.Rule, "image."):
		p.Image = issue.Subject
	default:
		p.Field = issue.Subject
	}
	p.Rule = "doctor." + p.Rule
	return p
}
