package process

import (
	"os/exec"
	"syscall"
)

// Lookup resolves name as os/exec.LookPath does in the inherited PATH.
func (*Adapter) Lookup(name string) (string, error) { return lookup(name, exec.LookPath) }

// Exec replaces the process with path through one execve, with the
// environment the process was given.
func (*Adapter) Exec(path string, argv []string) error {
	return replace(path, argv, syscall.Environ(), syscall.Exec) //nolint:gosec // the init's purpose is to execute the workload argv it was given
}
