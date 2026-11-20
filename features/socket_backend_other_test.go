//go:build !(linux && amd64)

package features

import (
	"os/exec"
	"testing"
)

// The kernel answer is chosen at the system call of a Linux amd64 process. The
// package still builds elsewhere, for the manual macOS and Windows jobs.

const socketBackendSupported = false

func socketBackendChild() {}

func runSocketBackendFixture(*testing.T) {}

// landlockRun is the supervision of one invocation. Nothing supervises here.
type landlockRun struct{}

func (w *world) superviseLandlock(cmd *exec.Cmd, umask int) (*landlockRun, error) {
	return nil, nil
}

func (r *landlockRun) end(started bool) error { return nil }

// landlockHelperError is the helper's own report that it could not run the program.
type landlockHelperError struct{}

func (e *landlockHelperError) Error() string { return "" }
