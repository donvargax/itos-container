package port

import (
	"context"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"strconv"
	"strings"
)

const (
	CredentialRefused     Outcome = "credential-refused"     //nolint:gosec // G101: diagnostic code, not a secret.
	CredentialUnavailable Outcome = "credential-unavailable" //nolint:gosec // G101: diagnostic code, not a secret.
)

type CredentialKinds struct{ Claude, GitHub bool }

func (k CredentialKinds) List() []string {
	var values []string
	if k.Claude {
		values = append(values, "claude-token")
	}
	if k.GitHub {
		values = append(values, "github-token")
	}
	return values
}
func CredentialIndex(kind string) int {
	if kind == "claude-token" {
		return 0
	}
	if kind == "github-token" {
		return 1
	}
	return -1
}
func CredentialTarget(kind string) string {
	if CredentialIndex(kind) < 0 {
		return ""
	}
	return "/run/itos-container/credentials/" + kind
}

// CredentialStaging retains pathname and inode authority, never source bytes.
type CredentialStaging struct {
	Directory string          `json:"directory"`
	Device    string          `json:"device"`
	Inode     string          `json:"inode"`
	Stage     string          `json:"stage"`
	Kinds     CredentialKinds `json:"kinds"`
	Files     [2]string       `json:"files"`
}
type CredentialMount struct{ Kind, Source, Target string }

func (s CredentialStaging) Mounts() []CredentialMount {
	var values []CredentialMount
	for _, kind := range s.Kinds.List() {
		values = append(values, CredentialMount{kind, s.Directory + "/" + kind, CredentialTarget(kind)})
	}
	return values
}
func credentialNumber(s string) bool {
	n, e := strconv.ParseUint(s, 10, 64)
	return e == nil && n > 0 && strconv.FormatUint(n, 10) == s
}
func (s CredentialStaging) Valid() bool {
	token := strings.TrimPrefix(s.Directory, "/dev/shm/itos-container-")
	if s.Directory != "/dev/shm/itos-container-"+token || len(token) != 32 || strings.Trim(token, "0123456789abcdef") != "" || len(s.Kinds.List()) == 0 {
		return false
	}
	if s.Stage == "intent" || s.Stage == "absent" && s.Device == "" && s.Inode == "" {
		return s.Device == "" && s.Inode == "" && s.Files == [2]string{}
	}
	for index, id := range s.Files {
		if id == "" {
			continue
		}
		parts := strings.Split(id, ":")
		if len(parts) != 2 || !credentialNumber(parts[0]) || !credentialNumber(parts[1]) || index == 0 && !s.Kinds.Claude || index == 1 && !s.Kinds.GitHub {
			return false
		}
	}
	return (s.Stage == "owned" || s.Stage == "absent") && credentialNumber(s.Device) && credentialNumber(s.Inode)
}
func (s CredentialStaging) Ready() bool {
	return s.Valid() && s.Stage == "owned" && (!s.Kinds.Claude || s.Files[0] != "") && (!s.Kinds.GitHub || s.Files[1] != "")
}

// CheckedCredential names a retained metadata descriptor in this process.
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

// Stage publishes directory and file identities before copying. Callers keep
// sources and the session lock until it returns, including after cancellation.
type CredentialStager interface {
	Stage(context.Context, Record, CredentialSources, func(CredentialStaging) Outcome) Outcome
	Observe(context.Context, Record) (string, Outcome)
	Remove(context.Context, Record) Outcome
}

func (r Record) ValidCredentials(b Binding) bool {
	if r.Schema != 4 {
		return r.Credentials == (CredentialStaging{}) && (b.Credentials == (CredentialKinds{}) || r.Stage == "reserved" || r.Stage == "cancelled")
	}
	return r.CredentialShape() && r.Credentials.Kinds == b.Credentials
}
func (r Record) credentialTransition(n Record) bool {
	a, b := r.Credentials, n.Credentials
	if r.Schema != 4 {
		if n.Schema != 4 {
			return a == (CredentialStaging{}) && b == (CredentialStaging{})
		}
		return (r.Schema == 2 || r.Schema == 3) && r.Stage == "reserved" && n.Stage == r.Stage && r.ContainerID == "" && n.ContainerID == "" && r.Anchor == n.Anchor && r.Egress == n.Egress && r.Anchor.Stage == "owned" && a == (CredentialStaging{}) && b.Valid() && b.Stage == "intent"
	}
	if n.Schema != 4 || !a.Valid() || !b.Valid() {
		return false
	}
	expected := a
	expected.Stage, expected.Device, expected.Inode, expected.Files = b.Stage, b.Device, b.Inode, b.Files
	if expected != b {
		return false
	}
	for index, id := range a.Files {
		if id != "" && id != b.Files[index] || a.Stage == "absent" && id != b.Files[index] {
			return false
		}
	}
	if a.Files != b.Files {
		if r.Stage != "reserved" || n.Stage != r.Stage || a.Stage != "owned" || b.Stage != "owned" {
			return false
		}
		changed := 0
		for index, id := range a.Files {
			if id != b.Files[index] {
				changed++
			}
		}
		if changed != 1 {
			return false
		}
	}
	if a == b {
		return true
	}
	if a.Stage == "intent" {
		return (b.Stage == "owned" || b.Stage == "absent" && b.Device == "" && b.Inode == "") && r.Stage == n.Stage && r.ContainerID == n.ContainerID
	}
	return a.Stage == "owned" && (b.Stage == "owned" || b.Stage == "absent") && a.Device == b.Device && a.Inode == b.Inode
}
func (r Record) HasEgress() bool {
	return r.Schema == 3 || r.Schema == 4 && r.Egress != (EgressRecord{})
}
func (r Record) OwnsCredentialMounts(o Observation) bool {
	expected := [2]CredentialMount{}
	for _, m := range r.Credentials.Mounts() {
		// Mounts emits only the two kinds selected by CredentialKinds.List.
		expected[CredentialIndex(m.Kind)] = m
	}
	return o.CredentialMounts == expected && (r.Schema != 4 || o.CredentialExact)
}

func (r Record) CredentialShape() bool {
	if r.Schema != 4 {
		return r.Credentials == (CredentialStaging{})
	}
	s := r.Credentials
	if !s.Valid() {
		return false
	}
	if r.Stage == "intent" || r.Stage == "owned" {
		return s.Stage != "intent" && (!s.Kinds.Claude || s.Files[0] != "") && (!s.Kinds.GitHub || s.Files[1] != "")
	}
	return true
}
