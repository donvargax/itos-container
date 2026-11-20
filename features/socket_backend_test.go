package features

import (
	"os/exec"
)

// The socket-backend fixture (T-66). A world pins the answer the kernel gives
// to the Landlock version query, and names the runtime inventory the fixture
// daemon reports. The slices' steps call these helpers; the step texts stay
// with them.
//
// These are stubs: they start every program unsupervised, record nothing and
// accept every choice. TestSocketBackendFixture holds the real behaviour.

// landlockNoUmask asks the wrapper to leave the program's umask as it is.
const landlockNoUmask = -1

// landlockCall is one landlock_create_ruleset call a supervised program made,
// once its answer was delivered.
type landlockCall struct {
	Version bool   // the call was the version query
	Answer  string // what was delivered: "ABI <n>", "ENOSYS", "EOPNOTSUPP" or "EPERM"
}

// landlockRun is the supervision of one invocation.
type landlockRun struct{}

func (w *world) pinKernelAnswer(text string) error { return nil }

func (w *world) onNextLandlockQuery(action func() error) error { return nil }

func (w *world) landlockRecord() [][]landlockCall { return nil }

func (w *world) closeLandlock() error { return nil }

func (w *world) chooseRuntimeInventory(name string) error { return nil }

func (w *world) superviseLandlock(cmd *exec.Cmd, umask int) (*landlockRun, error) {
	return nil, nil
}

func (r *landlockRun) end(started bool) error { return nil }
