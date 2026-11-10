package port

import "time"

// Checkout metadata inventories share these limits across readiness and source
// inspection. Reaching the deadline refuses; count and depth limits are inclusive.
const (
	MetadataBatchSize  = 128
	MetadataEntryLimit = 100000
	MetadataDepthLimit = 64
	MetadataBudget     = 10 * time.Second
)

// CredentialHandle pins an observed filesystem object until Close. Zero is the
// filesystem root's parent; Open(0, "/") starts a no-follow component walk.
type CredentialHandle uint64

// CredentialNode is metadata only. Mode uses the Unix file type and mode bits;
// Device and Inode retain their full native widths. No source bytes cross this port.
type CredentialNode struct {
	Device, Inode, Links uint64
	UID, Mode            uint32
	Size                 int64
}

type CredentialRead uint8

const (
	CredentialReadOK CredentialRead = iota
	CredentialReadMissing
	CredentialReadUnavailable
)

// CredentialDescriptor is a descriptor's independently observed mapping. MountID
// comes from statx STATX_MNT_ID; zero reports that required kernel evidence is
// unavailable, distinct from an unreadable pathname or inconsistent procfs data.
type CredentialDescriptor struct {
	Path    string
	MountID int
}

type CredentialMount struct {
	ID          int
	Device      uint64
	Root, Point string
	// Filesystem is mountinfo's type, needed to refuse stacking/pass-through
	// views whose backing paths cannot be derived from Root and Point.
	Filesystem string
	// Unprojectable retains a valid mount row whose root is not a filesystem
	// path (for example nsfs or a deleted bind). It supplies no coordinate.
	Unprojectable bool
}

// CredentialFS supplies raw, pinned metadata. Open never follows a symbolic
// link or opens file data. Entries reads at most 128 names per call, without
// following links, and maintains one directory cursor per handle. Mounts must
// return a complete, bounded and validated table, or false, never a prefix.
type CredentialFS interface {
	Open(CredentialHandle, string) (CredentialHandle, CredentialNode, CredentialRead)
	Stat(CredentialHandle) (CredentialNode, bool)
	Descriptor(CredentialHandle) (CredentialDescriptor, bool)
	Entries(CredentialHandle) ([]string, bool, bool)
	Mounts() ([]CredentialMount, bool)
	Now() time.Time
	Close(CredentialHandle)
}

// AccountState is the fixed account-home control tree, independent of State.
// Both boundaries are required whenever grants are inspected.
type CredentialInput struct {
	Root, State, AccountState, OS, Architecture string
	UID                                         int64
}

// CredentialIssue locates one first failure at the request's written index.
// Negative indices identify a shared boundary refusal rather than a source.
type CredentialIssue struct {
	Rule    string
	Index   int
	Subject string
}

type CredentialResult struct{ Issues []CredentialIssue }
