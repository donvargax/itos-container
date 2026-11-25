//go:build !(linux && amd64)

package features

import (
	"os/exec"
	"strings"
	"testing"
)

// The kernel answer is chosen at the system call of a Linux amd64 process. The
// package still builds elsewhere, for the manual macOS and Windows jobs.

const socketBackendSupported = false

func socketBackendChild() {}

func runSocketBackendFixture(*testing.T) {}

// TestSocketBackendPinRefusedWhereUnsupported holds the rule the fixture keeps
// where nothing supervises: a world cannot pin an answer, so runEnv never
// starts its program unfiltered while it reads the pin as supervision, and
// never drops the approval umask that its /bin/sh wrapper sets.
func TestSocketBackendPinRefusedWhereUnsupported(t *testing.T) {
	w := &world{}
	err := w.pinKernelAnswer("ABI 8")
	if err == nil {
		t.Fatal("pinning an answer succeeded on a host that cannot supervise it")
	}
	if !strings.Contains(err.Error(), "socket backend fixture") {
		t.Errorf("the refusal does not name the fixture: %v", err)
	}
	if w.landlock != nil {
		t.Error("the refused pin left the world pinned")
	}
}

// landlockRun is the supervision of one invocation. Nothing supervises here, so
// pinKernelAnswer refuses and no world reaches superviseLandlock pinned.
type landlockRun struct{}

func (w *world) superviseLandlock(cmd *exec.Cmd, umask int) (*landlockRun, error) {
	return nil, nil
}

func (r *landlockRun) end(started bool) error { return nil }

// landlockHelperError is the helper's own report that it could not run the program.
type landlockHelperError struct{}

func (e *landlockHelperError) Error() string { return "" }
