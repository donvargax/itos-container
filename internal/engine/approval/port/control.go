package port

import "context"

// Account resolves the effective host account independently of the environment.
type Account interface{ AccountHome() (string, Failure) }

// Control serializes location publication and worker registration for one UID.
// Lock waits respect the caller deadline and a maximum 10-second call budget;
// contention returns state.busy. A read transaction never creates or syncs state. Writers
// require prior consent or independently validated existing approval/ownership.
type Control interface {
	Begin(context.Context, Candidate, bool) (ControlTransaction, Failure)
}
type ControlTransaction interface {
	Unchanged() bool
	Close()
}

// Reservations validates all launch authority in a selected managed tree,
// including legacy public records and their original intent/snapshot bindings.
// It takes no operation lock and performs no engine I/O or repair.
type Reservations interface {
	Roots(context.Context, Candidate, string) ([]string, Failure)
}

// Locations exposes protected read-only discovery without importing adapters.
type Locations interface {
	At(string) Storage
	Inspect(context.Context, Candidate, string) (AdmissionDirectory, Failure)
}
