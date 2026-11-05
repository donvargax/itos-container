package port

import (
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"strings"
)

// Problem is the stable public diagnostic contract for this inspection.
type Problem struct {
	File    string  `json:"file,omitempty"`
	Label   string  `json:"label,omitempty"`
	Rule    string  `json:"rule"`
	Exit    int     `json:"exit"`
	Message string  `json:"message"`
	Fix     string  `json:"fix"`
	Path    *string `json:"path,omitempty"`
	Field   string  `json:"field,omitempty"`
	Image   string  `json:"image,omitempty"`
}

// Diagnostic maps a typed issue without exposing rejected source values.
func Diagnostic(issue Issue) Problem {
	p := Problem{Exit: 1}
	switch issue.Kind {
	case HostUnsupported:
		p.Rule, p.Message, p.Fix = "host.unsupported", "Host is outside the inspection target", "use Linux amd64 with native Docker"
	case CheckoutRoot:
		p.Rule, p.Exit, p.Message, p.Fix = "checkout.root", 2, "Checkout root is not absolute", "set ITOS_ROOT to the absolute checkout root"
	case CheckoutUnavailable:
		p.Rule, p.Exit, p.Message, p.Fix = "checkout.unavailable", 3, "Checkout root is unavailable", "provide a readable checkout directory"
	case CheckoutLayout:
		p.Rule, p.Message, p.Fix = "checkout.layout", "Checkout layout is unsupported", "use a standalone checkout with embedded HEAD, config, objects and refs and no external Git storage"
	case CheckoutPath:
		p.Rule, p.Message, p.Fix = "checkout.path", "Checkout inspection path is unsafe", "use real root directories and Git metadata without symbolic links"
	case CheckoutChanged:
		p.Rule, p.Message, p.Fix = "checkout.changed", "Checkout identity changed during inspection", "stabilize the checkout and inspect again"
	case EngineRemote:
		p.Rule, p.Message, p.Fix = "engine.remote", "Remote engine transports are unsupported", "select a host-owned local Unix socket"
	case EngineSelection:
		p.Rule, p.Exit, p.Message, p.Fix = "engine.selection", 2, "Engine selection is unsupported", "use only DOCKER_HOST=unix:///absolute/socket without context, TLS or API overrides"
	case EngineUntrusted:
		p.Rule, p.Message, p.Fix = "engine.untrusted", "Engine socket belongs to the checkout", "select a host-owned socket outside the checkout"
	case EngineUnavailable:
		p.Rule, p.Exit, p.Message, p.Fix = "engine.unavailable", 3, "Local engine socket is unavailable", "have the host operator check the socket and its permissions"
	case EngineIdentityMissing:
		p.Rule, p.Message, p.Fix = "engine.identity", "Engine supplied no daemon identity", "use a native Docker engine that reports its daemon ID"
	case EngineAPI:
		p.Rule, p.Message, p.Fix = "engine.api", "Engine and client API ranges are incompatible", "use an engine supporting the pinned client's negotiated API"
	case EngineUnsupported:
		p.Rule, p.Message, p.Fix = "engine.unsupported", "Engine product is outside the inspection target", "use native Docker Engine Community or Enterprise"
	case EnginePlatform:
		p.Rule, p.Message, p.Fix = "engine.platform", "Engine platform observations are unsupported or inconsistent", "use a native Linux amd64 Docker engine"
	case EngineTimeout:
		p.Rule, p.Exit, p.Message, p.Fix = "engine.timeout", 75, "Engine inspection exceeded five seconds", "restore engine responsiveness before inspecting again"
	case EngineInspect:
		p.Rule, p.Exit, p.Message, p.Fix = "engine.inspect", 75, "Engine inspection failed", "have the host operator check engine health before inspecting again"
	case EngineChanged:
		p.Rule, p.Message, p.Fix = "engine.changed", "Engine identity changed during inspection", "stabilize the selected daemon and socket before inspecting again"
	case ImageMissing:
		p.Rule, p.Exit, p.Message, p.Fix = "image.missing", 3, "Requested image is not cached", "ask the host operator to prepare the exact image outside doctor"
	case ImageInspect:
		p.Rule, p.Exit, p.Message, p.Fix = "image.inspect", 75, "Cached image inspection was unusable", "have the host operator check the exact cached image and engine health"
	case ImageDigest:
		p.Rule, p.Message, p.Fix = "image.digest", "Cached image does not attest the requested repository digest", "ask the host operator to prepare the exact image outside doctor"
	case ImagePlatform:
		p.Rule, p.Message, p.Fix = "image.platform", "Cached image platform is incompatible", "prepare the exact requested Linux amd64 image outside doctor"
	case ImageUser:
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

// Problems preserves the policy/manifest/readiness rules through every consumer.
func Problems(result Result) []Problem {
	out := []Problem{}
	if result.Policy != nil {
		for _, issue := range result.Policy.Issues {
			p := policy.Diagnostic(issue)
			out = append(out, Problem{Rule: p.Rule, Exit: p.Exit, Message: p.Message, Fix: p.Fix, File: p.File, Path: &p.Path})
		}
	}
	if result.Manifest != nil {
		for _, issue := range result.Manifest.Issues {
			p := manifest.Diagnostic(*result.Manifest, issue)
			out = append(out, Problem{Rule: p.Rule, Exit: p.Exit, Message: p.Message, Fix: p.Fix, File: p.File, Path: &p.Path, Image: p.Image, Label: p.Label})
		}
	}
	for _, issue := range result.Issues {
		out = append(out, Diagnostic(issue))
	}
	return out
}
