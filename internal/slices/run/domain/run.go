// Package domain validates an invocation before any startup effect.
package domain

import "errors"

var (
	ErrJSONUnsupported = errors.New("run JSON is unsupported")
	ErrCommandRequired = errors.New("run command is required")
)

// Invocation separates the wrapper request from literal workload arguments.
type Invocation struct {
	JSON      bool
	Separated bool
	Argv      []string
}

// Refuse gives JSON precedence over argv validation before preflight or startup.
func Refuse(request Invocation) error {
	if request.JSON {
		return ErrJSONUnsupported
	}
	if !request.Separated || len(request.Argv) == 0 || request.Argv[0] == "" {
		return ErrCommandRequired
	}
	return nil
}
