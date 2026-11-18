package port

import "context"

// SessionLogs stages the one log a registered session's directory holds beside
// its record, egress-refusals.jsonl. The string is the public session ID and the
// candidate is the one RegistryStorage.Open takes for that session. Stage neither
// takes nor checks the session's operation lock, which its caller holds. A
// session that was never registered stages nothing: nil and no failure.
type SessionLogs interface {
	Stage(context.Context, Candidate, string) (StagedLog, Failure)
	// Evidence reads a session's published log and its report through the same
	// pinned directory Stage publishes into. Absent files are nil with no
	// failure; Path names the log's protected destination.
	Evidence(ctx context.Context, c Candidate, id string) (SessionEvidence, Failure)
}

// SessionEvidence is a session's durably published refusal log pair. Log is
// egress-refusals.jsonl and Report the committed copy's own report beside it.
type SessionEvidence struct {
	Path   string
	Log    []byte
	Report []byte
}

// StagedLog is a private temporary that only Publish makes visible, by one
// rename of a file already synced. A failed Write poisons it. Publish and Close
// alone release it, each on every return. It serves one caller at a time.
type StagedLog interface {
	// Write stages the log's bytes; Attach stages the report published beside
	// it. Attach must precede Publish and cannot follow a failed Write.
	Write([]byte) Failure
	Attach([]byte) Failure
	// Path names the destination Publish renames the staged log to.
	Path() string
	Publish() Failure
	Close()
}
