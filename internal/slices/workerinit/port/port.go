// Package port declares what the trusted worker init needs from the kernel:
// the Landlock restriction and the process replacement. It performs no I/O.
package port

import "errors"

// ErrUnavailable means the kernel or platform cannot do what the call asks:
// Landlock is missing or disabled, or the program is not running on Linux.
var ErrUnavailable = errors.New("unavailable")

// ErrNotFound means the program the workload names does not exist, or cannot
// be resolved to an absolute path.
var ErrNotFound = errors.New("not found")

// Landlock reads the kernel's Landlock ABI and restricts the calling thread.
type Landlock interface {
	// ABI returns the kernel's Landlock ABI version. Where Landlock is missing
	// or disabled the error matches ErrUnavailable.
	ABI() (int, error)
	// Restrict forbids the calling thread, and everything it starts, to
	// connect or send to a pathname socket created outside its domain. It
	// holds no weaker mode: a kernel without that right returns an error.
	Restrict() error
}

// Process finds and replaces the running program.
type Process interface {
	// Lookup resolves the program a workload names. A name with a slash is
	// returned unchanged. A bare name is searched in the inherited PATH and
	// must resolve to an absolute path, or the error matches ErrNotFound.
	Lookup(name string) (string, error)
	// Exec replaces the process with path, argv exactly as given and the
	// environment exactly as inherited. It returns only when it failed: a
	// missing path matches ErrNotFound.
	Exec(path string, argv []string) error
}
