package port

import "context"

// SessionLogs stages the one log a registered session's directory holds beside
// its record, egress-refusals.jsonl. The string is the public session ID and the
// candidate is the one RegistryStorage.Open takes for that session. Stage neither
// takes nor checks the session's operation lock, which its caller holds. A
// session that was never registered stages nothing: nil and no failure.
type SessionLogs interface {
	Stage(context.Context, Candidate, string) (StagedLog, Failure)
}

// StagedLog is a private temporary that only Publish makes visible, by one
// rename of a file already synced. A failed Write poisons it. Publish and Close
// alone release it, each on every return. It serves one caller at a time.
type StagedLog interface {
	Write([]byte) Failure
	Publish() Failure
	Close()
}
