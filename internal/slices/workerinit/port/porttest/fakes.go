// Package porttest holds the in-memory fakes and the shared contracts of the
// worker init's ports. Nothing here touches the kernel.
package porttest

import (
	"errors"
	"strings"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
)

// Recorder keeps the order in which fakes were called.
type Recorder struct{ Calls []string }

// Landlock is a port.Landlock whose kernel is a number: Version is the ABI
// it reports, and Restrict fails below ABI 9 as the real kernel does.
type Landlock struct {
	Rec         *Recorder
	Version     int
	VersionErr  error
	RestrictErr error
}

// ABI records the call and reports the configured version or error.
func (l *Landlock) ABI() (int, error) {
	l.Rec.Calls = append(l.Rec.Calls, "abi")
	return l.Version, l.VersionErr
}

// Restrict records the call. It fails when configured to, and below ABI 9.
func (l *Landlock) Restrict() error {
	l.Rec.Calls = append(l.Rec.Calls, "restrict")
	if l.VersionErr != nil {
		return l.VersionErr
	}
	if l.RestrictErr != nil {
		return l.RestrictErr
	}
	if l.Version < MinABI {
		return errors.New("fake kernel handles no pathname socket right")
	}
	return nil
}

// Process is a port.Process over a map of the programs that exist.
type Process struct {
	Rec *Recorder
	// Bare maps a bare name to the absolute path PATH resolves it to.
	Bare map[string]string
	// Files holds the paths Exec can find.
	Files map[string]bool
	// LookupErr and ExecErr, when set, replace the outcome.
	LookupErr, ExecErr error

	Looked   []string
	ExecPath string
	ExecArgv []string
}

// Lookup records the name and resolves it from Bare. A name with a slash
// comes back unchanged.
func (p *Process) Lookup(name string) (string, error) {
	p.Rec.Calls = append(p.Rec.Calls, "lookup")
	p.Looked = append(p.Looked, name)
	if p.LookupErr != nil {
		return "", p.LookupErr
	}
	if strings.Contains(name, "/") {
		return name, nil
	}
	path, ok := p.Bare[name]
	if !ok {
		return "", port.ErrNotFound
	}
	return path, nil
}

// Exec records what it was given and returns, as a failed execve does. A
// path in no file is not found.
func (p *Process) Exec(path string, argv []string) error {
	p.Rec.Calls = append(p.Rec.Calls, "exec")
	p.ExecPath = path
	p.ExecArgv = argv
	if p.ExecErr != nil {
		return p.ExecErr
	}
	if !p.Files[path] {
		return port.ErrNotFound
	}
	return nil
}
