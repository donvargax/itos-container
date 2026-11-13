// Package executable observes the controller's own program without running it.
package executable

import "github.com/donvargax/itos-container/internal/engine/readiness/port"

// Program reads the file Path names; nil is the process's own executable.
// Slice-27's step checkpoint binds only this shape: until the ELF reading is
// built every observation fails, which the domain never reads as safe.
type Program struct {
	Path func() (string, error)
}

var _ port.Executables = Program{}

// Observe reports that the program could not be observed.
func (Program) Observe() (port.Executable, port.Failure) {
	return port.Executable{}, port.EgressExecutable
}
