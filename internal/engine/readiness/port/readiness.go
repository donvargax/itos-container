// Package port owns read-only checkout and container-engine observations.
package port

import (
	"context"
	"time"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
)

// Failure identifies a refused prerequisite without transporting raw errors.
type Failure uint8

const (
	None Failure = iota
	HostUnsupported
	CheckoutRoot
	CheckoutUnavailable
	CheckoutLayout
	CheckoutPath
	CheckoutChanged
	CheckoutHardlink
	EngineRemote
	EngineSelection
	EngineUntrusted
	EngineUnavailable
	EngineIdentityMissing
	EngineAPI
	EngineUnsupported
	EnginePlatform
	EngineTimeout
	EngineInspect
	EngineChanged
	ImageMissing
	ImageInspect
	ImageDigest
	ImagePlatform
	ImageUser
	// EgressExecutable refuses the controller's own program as the proxy's:
	// unobserved, not a static Linux amd64 ELF, or inside the checkout.
	EgressExecutable
)

// Issue contains only a typed failure and its safe subject, never source data.
type Issue struct {
	Kind    Failure
	Subject string
}

// Input carries host observations and selection values, supplied by assembly.
type Input struct {
	OS, Architecture, Root                         string
	Host, Context, TLSVerify, CertPath, APIVersion string
}

// CheckoutIdentity binds the selected path to its root and embedded Git directory.
// Device and inode values are decimal strings to preserve all their bits.
type CheckoutIdentity struct{ Root, Device, Inode, GitDevice, GitInode string }

// Checkout pins directories before readers use them. Inspect requires every
// regular file recursively below the root (including Git and ignored files) to
// have exactly one filesystem link. Unavailable metadata/traversal fails closed.
// Unchanged rechecks that independence as well as root/Git identity; approval
// never caches layout safety. Recovery need not re-admit mutable checkout data.
// Close releases the pin. A failed Pin never leaves a usable pin behind.
type Checkout interface {
	Pin(root string) Failure
	Inspect() (CheckoutIdentity, Failure)
	Unchanged() bool
	Close()
}

// Engine opens only a validated host-owned local socket. The budget covers the
// whole session, including negotiation and every subsequent request; no retries.
type Engine interface {
	Open(selection, checkout string, budget time.Duration) (Session, Failure)
}

// Session exposes only inspection; there is no acquisition or mutation method.
type Session interface {
	Endpoint() string
	Version() (Version, Failure)
	Info() (Info, Failure)
	Image(reference string) (Image, Failure)
	Unchanged() bool
	Close()
}

// Version retains API ranges separately from the actually negotiated version.
type Version struct {
	Product, OS, Architecture                            string
	Minimum, Maximum, Negotiated, SDKMinimum, SDKMaximum string
}

// Info contains only identity and platform observations used by the domain.
type Info struct{ ID, OS, Architecture, OperatingSystem string }

// Image contains the inspected fields needed for readiness. ConfigPresent
// distinguishes an absent Config from its valid empty fields. Label stays data.
type Image struct {
	ID, OS, Architecture, User string
	RepoDigests                []string
	ConfigPresent              bool
	Label                      manifest.Label
}

// Digests maps a named sha256 reference to its Docker-normalized repository and
// digest, omitting any tag. Invalid or undigested references return an empty
// string. It performs no lookup and never changes the request used for reports.
type Digests interface{ Canonical(reference string) string }

// EngineIdentity and ImageIdentity are the observations safe to report.
type EngineIdentity struct{ Endpoint, ID, APIVersion, OS, Architecture string }
type ImageIdentity struct{ Reference, ID, OS, Architecture string }

// Result is a snapshot, not approval or runtime isolation evidence.
type Result struct {
	Checkout *CheckoutIdentity
	Engine   *EngineIdentity
	Image    *ImageIdentity
	Policy   *policy.Result
	Manifest *manifest.Result
	Issues   []Issue
	Ready    bool
}

// ContextEngine separates establishment from the retained inspection lifetime.
// Run supplies each finite request context; legacy callers retain Open's total.
type ContextEngine interface {
	OpenContext(context.Context, string, string) (Session, Failure)
}
type ContextSession interface{ ForContext(context.Context) Session }
