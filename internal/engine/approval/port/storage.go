package port

import "context"

// Storage shares the approval root's pinned, no-follow protections with its
// snapshots and ownership intents. It does not resolve current approval.
type Storage interface {
	Open(context.Context, Candidate, string, bool) (Directory, Failure)
}

// Directory is locked until Close. Publish compares the exact preceding bytes;
// nil means exclusive creation. Sync re-establishes durability without rewriting.
type Directory interface {
	Read(string) ([]byte, Failure)
	Publish(string, []byte, []byte) Failure
	Sync(string, []byte) Failure
	Unchanged() bool
	Close()
}
