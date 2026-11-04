// Package domain validates an invocation before any startup capability exists.
package domain

import "errors"

var (
	ErrJSONUnsupported    = errors.New("run JSON is unsupported")
	ErrCommandRequired    = errors.New("run command is required")
	ErrStartupUnavailable = errors.New("offline startup is unavailable")
)

// Invocation separates the wrapper request from literal workload arguments.
type Invocation struct {
	JSON      bool
	Separated bool
	Argv      []string
}

// Refuse gives JSON precedence over argv validation. Even a valid invocation
// refuses until the preflight and owned-startup slices supersede this boundary.
// No filesystem, policy or engine capability is available to this domain.
func Refuse(request Invocation) error {
	if request.JSON {
		return ErrJSONUnsupported
	}
	if !request.Separated || len(request.Argv) == 0 || request.Argv[0] == "" {
		return ErrCommandRequired
	}
	return ErrStartupUnavailable
}
