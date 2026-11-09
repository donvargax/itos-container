// Package domain checks read-only readiness without granting permission to run.
package domain

import (
	"strings"
	"time"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/domain"
	manifestport "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/domain"
	policyport "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Inspector's file readers must use the checkout pin established by Pin.
// Assembly binds that capability; file-only inspection does not depend on it.
type Inspector struct {
	Checkout   port.Checkout
	Policy     policyport.Source
	Names      policyport.Names
	Files      manifestport.Files
	Parser     manifestport.Parser
	References manifestport.References
	Digests    port.Digests
	Engine     port.Engine
}

// Inspect stops at unsafe prerequisites and retains only facts actually read.
func (i Inspector) Inspect(input port.Input) port.Result { return i.InspectBeforeEngine(input, nil) }

// InspectBeforeEngine retains the checkout pin across the local checkpoint and
// every subsequent engine observation. A refusal never opens an engine session.
func (i Inspector) InspectBeforeEngine(input port.Input, check func(port.Result) bool) port.Result {
	result := port.Result{}
	if failure := i.Checkout.Pin(input.Root); failure != port.None {
		result.Issues = []port.Issue{{Kind: failure, Subject: input.Root}}
		return result
	}
	defer i.Checkout.Close()
	p := policy.Inspect(i.Policy, i.Names)
	if p.Policy != nil && p.Policy.Version != 1 {
		// Canonical settings v1 hardcode an offline profile: recording them for a
		// request that asks for network would approve what was never reviewed.
		// Only doctor --policy-only reads such a file until approval can bind it.
		p = policyport.Result{Issues: []policyport.Issue{{Kind: policyport.Version, Path: []string{"version"}, Version: p.Policy.Version}}}
	}
	result.Policy = &p
	if p.Policy == nil || len(p.Issues) != 0 {
		return result
	}
	m := manifest.Inspect(p.Policy.Manifest, i.Files, i.Parser, i.References)
	result.Manifest = &m
	if m.Effective == nil || len(m.Issues) != 0 {
		return result
	}
	if input.OS != "linux" || input.Architecture != "amd64" {
		result.Issues = []port.Issue{{Kind: port.HostUnsupported, Subject: "host"}}
		return result
	}
	checkout, failure := i.Checkout.Inspect()
	if failure != port.None {
		result.Issues = []port.Issue{{Kind: failure, Subject: input.Root}}
		return result
	}
	result.Checkout = &checkout
	if field := alternateSelection(input); field != "" {
		result.Issues = []port.Issue{{Kind: port.EngineSelection, Subject: field}}
		return result
	}
	if check != nil && !check(result) {
		return result
	}
	session, failure := i.Engine.Open(input.Host, checkout.Root, 5*time.Second)
	if failure != port.None {
		result.Issues = []port.Issue{{Kind: failure, Subject: "DOCKER_HOST"}}
		return result
	}
	defer session.Close()
	i.inspectEngine(&result, session)
	if !i.Checkout.Unchanged() {
		result.Issues = append(result.Issues, port.Issue{Kind: port.CheckoutChanged, Subject: checkout.Root})
	}
	result.Ready = len(result.Issues) == 0 && len(result.Manifest.Issues) == 0
	return result
}

func alternateSelection(input port.Input) string {
	for _, setting := range []struct{ key, value string }{
		{"DOCKER_CONTEXT", input.Context}, {"DOCKER_TLS_VERIFY", input.TLSVerify},
		{"DOCKER_CERT_PATH", input.CertPath}, {"DOCKER_API_VERSION", input.APIVersion},
	} {
		if setting.value != "" {
			return setting.key
		}
	}
	return ""
}

func (i Inspector) inspectEngine(result *port.Result, session port.Session) {
	fail := func(kind port.Failure) {
		result.Issues = append(result.Issues, port.Issue{Kind: kind, Subject: "DOCKER_HOST"})
	}
	version, failure := session.Version()
	if failure != port.None {
		fail(failure)
		return
	}
	if !compatibleAPI(version) {
		fail(port.EngineAPI)
		return
	}
	info, failure := session.Info()
	if failure != port.None {
		fail(failure)
		return
	}
	if failure = engineFailure(version, info); failure != port.None {
		fail(failure)
		return
	}
	result.Engine = &port.EngineIdentity{Endpoint: session.Endpoint(), ID: info.ID, APIVersion: version.Negotiated, OS: info.OS, Architecture: architecture(info.Architecture)}
	request := result.Manifest.Effective
	image, failure := session.Image(request.Image)
	if failure == port.None && (!validImageID(image.ID) || !image.ConfigPresent) {
		failure = port.ImageInspect
	}
	if failure != port.None {
		result.Issues = append(result.Issues, port.Issue{Kind: failure, Subject: request.Image})
		return
	}
	result.Image = &port.ImageIdentity{Reference: request.Image, ID: image.ID, OS: image.OS, Architecture: image.Architecture}
	result.Issues = append(result.Issues, i.imageIssues(image, *request)...)
	validated := manifest.WithMetadata(*result.Manifest, image.Label, i.Parser)
	result.Manifest = &validated
	if !session.Unchanged() {
		fail(port.EngineChanged)
		return
	}
	current, failure := session.Info()
	if failure != port.None {
		fail(failure)
		return
	}
	if current.ID != info.ID || !session.Unchanged() {
		fail(port.EngineChanged)
	}
}

func engineFailure(version port.Version, info port.Info) port.Failure {
	if strings.TrimSpace(info.ID) == "" {
		return port.EngineIdentityMissing
	}
	if version.Product != "Docker Engine - Community" && version.Product != "Docker Engine - Enterprise" {
		return port.EngineUnsupported
	}
	for _, name := range []string{"docker desktop", "rancher", "podman"} {
		if strings.Contains(strings.ToLower(info.OperatingSystem), name) {
			return port.EngineUnsupported
		}
	}
	if version.OS != "linux" || info.OS != "linux" || architecture(version.Architecture) != "amd64" || architecture(info.Architecture) != "amd64" {
		return port.EnginePlatform
	}
	return port.None
}

func architecture(value string) string {
	if value == "x86_64" {
		return "amd64"
	}
	return value
}

// ValidateEngine applies the same negotiated-API/native-platform rules for
// recovery as readiness, without re-reading mutable startup configuration.
func ValidateEngine(version port.Version, info port.Info) port.Failure {
	if !compatibleAPI(version) {
		return port.EngineAPI
	}
	return engineFailure(version, info)
}
