// Package landlock is the Linux adapter of the worker init's Landlock port.
//
// The order of the raw calls and their cleanup live here, over an unexported
// set of raw-call functions. The _linux.go file supplies the real ones, and
// every other platform answers port.ErrUnavailable.
package landlock

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
)

// rulesetAttr is the part of struct landlock_ruleset_attr the init sets.
type rulesetAttr struct {
	HandledAccessFS  uint64
	HandledAccessNet uint64
	Scoped           uint64
}

// calls are the raw system calls, and the two constants the _linux.go file
// takes from golang.org/x/sys/unix, so no number is copied here.
type calls struct {
	// handledFS is LANDLOCK_ACCESS_FS_RESOLVE_UNIX, the one right handled.
	handledFS uint64
	// noNewPrivs is PR_SET_NO_NEW_PRIVS.
	noNewPrivs int

	// version is landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION).
	version func() (int, error)
	// create is landlock_create_ruleset(attr, size, flags).
	create func(attr rulesetAttr, flags uint) (int, error)
	// prctl is prctl(option, arg2, arg3, arg4, arg5).
	prctl func(option int, arg2, arg3, arg4, arg5 uintptr) error
	// restrict is landlock_restrict_self(fd, flags).
	restrict func(fd int, flags uint) error
	// close closes a descriptor.
	close func(fd int) error
}

// Adapter is the port.Landlock over the kernel's Landlock system calls.
type Adapter struct{ calls calls }

// New returns the adapter over the real system calls of this platform.
func New() *Adapter { return &Adapter{calls: realCalls()} }

var _ port.Landlock = (*Adapter)(nil)

// ABI reads the kernel's Landlock ABI. A kernel without Landlock, or with it
// disabled, matches port.ErrUnavailable.
func (a *Adapter) ABI() (int, error) {
	abi, err := a.calls.version()
	if err != nil {
		return 0, classify(err)
	}
	return abi, nil
}

// classify makes the errors that mean no Landlock match port.ErrUnavailable
// and leaves every other error as it is.
func classify(err error) error {
	if errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EOPNOTSUPP) {
		return fmt.Errorf("%w: %w", port.ErrUnavailable, err)
	}
	return err
}
