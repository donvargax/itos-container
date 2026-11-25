// Package domain decides what the trusted worker init does before it hands
// the process to the workload: restrict first, resolve next, replace last.
package domain

import (
	"errors"
	"slices"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
)

// Code names why the init refused to start the workload.
type Code string

// The refusals a start can end in. Exec returning at all is a refusal.
const (
	Unavailable Code = "landlock-unavailable"
	Failed      Code = "landlock-failed"
	NotFound    Code = "not-found"
	ExecFailed  Code = "exec-failed"
)

// minABI is the first Landlock ABI that handles pathname sockets.
const minABI = 9

// Start restricts the calling thread, resolves argv[0] and replaces the
// process with the workload, in that order, each step at most once and never
// retried. It returns only the reason it did not: a replaced process never
// comes back. argv must hold at least the program.
//
// The caller must run it on one locked thread without starting another: the
// restriction belongs to the thread that makes it, and only the thread that
// executes hands it to the workload.
func Start(l port.Landlock, p port.Process, argv []string) Code {
	abi, err := l.ABI()
	switch {
	case errors.Is(err, port.ErrUnavailable):
		return Unavailable
	case err != nil:
		return Failed
	case abi < minABI:
		return Unavailable
	}
	if err := l.Restrict(); err != nil {
		return Failed
	}
	path, err := p.Lookup(argv[0])
	if err != nil {
		return NotFound
	}
	// The workload argv is passed on as an independent copy, never changed.
	if err := p.Exec(path, slices.Clone(argv)); errors.Is(err, port.ErrNotFound) {
		return NotFound
	}
	return ExecFailed
}
