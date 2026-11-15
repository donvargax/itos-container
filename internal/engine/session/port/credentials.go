package port

import (
	"context"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

const (
	CredentialRefused     Outcome = "credential-refused"     //nolint:gosec // G101: diagnostic code, not a secret.
	CredentialUnavailable Outcome = "credential-unavailable" //nolint:gosec // G101: diagnostic code, not a secret.
)

// CredentialKinds is a set, deliberately comparable and free of source bytes.
type CredentialKinds struct{ Claude, GitHub bool }

func (k CredentialKinds) List() []string {
	var kinds []string
	if k.Claude {
		kinds = append(kinds, "claude-token")
	}
	if k.GitHub {
		kinds = append(kinds, "github-token")
	}
	return kinds
}
func CredentialTarget(kind string) string {
	if kind != "claude-token" && kind != "github-token" {
		return ""
	}
	return "/run/itos-container/credentials/" + kind
}

// CredentialStaging is durable pathname authority, never a checked source or
// read buffer. The directory identity is published before the first file copy.
type CredentialStaging struct {
	Directory string          `json:"directory"`
	Device    string          `json:"device"`
	Inode     string          `json:"inode"`
	Stage     string          `json:"stage"`
	Kinds     CredentialKinds `json:"kinds"`
}
type CredentialMount struct{ Kind, Source, Target string }

func (s CredentialStaging) Mounts() []CredentialMount {
	var mounts []CredentialMount
	for _, kind := range s.Kinds.List() {
		mounts = append(mounts, CredentialMount{kind, s.Directory + "/" + kind, CredentialTarget(kind)})
	}
	return mounts
}
func (s CredentialStaging) Valid() bool { return false }

// CheckedCredential is the retained metadata-only source capability. ReadPath
// names the held descriptor in this process, never the policy pathname.
type CheckedCredential struct {
	Kind, ReadPath string
	Node           readiness.CredentialNode
}
type CredentialSources interface {
	Kinds() CredentialKinds
	Revalidate(context.Context) Outcome
	Checked() []CheckedCredential
	Close()
}
type CredentialEntropy interface{ StagingToken() (string, Outcome) }

// Stage consumes a complete, freshly revalidated source set. Its callback
// publishes the new directory identity before any source bytes are copied.
// Callers retain the session operation lock and sources until Stage returns,
// including after cancellation. Observe and Remove need no original sources.
type CredentialStager interface {
	Stage(context.Context, Record, CredentialSources, func(CredentialStaging) Outcome) Outcome
	Observe(context.Context, Record) (string, Outcome)
	Remove(context.Context, Record) Outcome
}

func (r Record) ValidCredentials(b Binding) bool { return false }
