package port

import "context"

// OriginalStorage reads only existing protected evidence for a known project.
// The caller must subsequently validate the decoded original checkout against
// the selected state root; mutable checkout discovery is not an input.
type OriginalStorage interface {
	ReadOriginal(context.Context, Candidate, string, string) ([]byte, Failure)
}
