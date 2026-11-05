// Package port owns read-only checkout and container-engine observations.
package port

import (
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

// Checkout pins directories before readers use them, then detects substitution.
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
