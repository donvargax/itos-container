// Package process is the adapter of the worker init's Process port: it
// resolves the workload's program as os/exec does and replaces the init with
// it in one execve, never forking.
//
// The decisions live here, untagged, over the call that does the work. The
// _linux.go file supplies the real calls and every other platform answers
// port.ErrUnavailable, since Go's syscall.Exec cannot replace a process on
// Windows.
package process

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
)

// Adapter is the port.Process over os/exec's PATH search and execve.
type Adapter struct{}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

var _ port.Process = (*Adapter)(nil)

// lookup returns a name with a slash unchanged and without touching the
// filesystem. Any other name is searched with look, as os/exec.LookPath does
// in the inherited PATH, and the result must be absolute: a relative one is
// not found, whether or not look reports exec.ErrDot, since
// GODEBUG=execerrdot=0 suppresses that error.
func lookup(name string, look func(string) (string, error)) (string, error) {
	if strings.Contains(name, "/") {
		return name, nil
	}
	path, err := look(name)
	if err != nil || !filepath.IsAbs(path) {
		return "", port.ErrNotFound
	}
	return path, nil
}

// replace is one execve of path with argv and env exactly as given. ENOENT
// is port.ErrNotFound; any other error returns as itself.
func replace(path string, argv, env []string, execve func(string, []string, []string) error) error {
	err := execve(path, argv, env)
	if errors.Is(err, syscall.ENOENT) {
		return fmt.Errorf("%w: %w", port.ErrNotFound, err)
	}
	return err
}
