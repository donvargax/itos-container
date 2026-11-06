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
	List(context.Context, Candidate) ([]string, Failure)
	Register(context.Context, Candidate, string, []byte) (ControllerLease, Failure)
	Open(context.Context, Candidate, string, bool) (SessionDirectory, Failure)
}
type ControllerLease interface{ Close() }
type SessionDirectory interface {
	Directory
	Controller() (string, Failure)
}
