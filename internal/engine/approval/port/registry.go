package port

import (
	"context"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

type ProjectIdentity interface {
	Project(readiness.CheckoutIdentity, string) string
}

// RegistryStorage reuses protected approval ancestors without consulting approval
// records. Persistent lock inodes live beside each public session record.
type RegistryStorage interface {
	Admission(context.Context, Candidate) (AdmissionDirectory, Failure)
	List(context.Context, Candidate) ([]string, Failure)
	Register(context.Context, Candidate, string, []byte) (ControllerLease, Failure)
	Open(context.Context, Candidate, string, bool) (SessionDirectory, Failure)
}
type ControllerLease interface{ Close() }
type SessionDirectory interface {
	Directory
	Controller() (string, Failure)
}

// AdmissionDirectory holds common account exclusion and the selected admission
// directory lock. Sessions reads protected
// public records without taking operation locks; no engine call may span this
// transaction. Its own files are durable reservation/completion certificates.
type AdmissionDirectory interface {
	Directory
	Names() ([]string, Failure)
	Sessions() ([]RegisteredSession, Failure)
}
type RegisteredSession struct {
	Index, ID string
	Data      []byte
}
