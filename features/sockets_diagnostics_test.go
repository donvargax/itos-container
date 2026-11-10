package features

import (
	"fmt"
	"strings"
)

// Report only controller output, request progress and engine failure/state
// fields. Full inspect/create bodies contain unrelated config and environment.
func (s *socketWorld) startupDiagnostics() string {
	o := s.w.offline
	var out strings.Builder
	fmt.Fprintf(&out, "controller stderr: %q\n", o.stderr)
	if o.api == nil {
		out.WriteString("engine recorder unavailable\n")
		return out.String()
	}
	a := o.api
	a.mu.Lock()
	defer a.mu.Unlock()
	fmt.Fprintf(&out, "engine requests: %d; recorder problem: %v; observation problem: %v\n", len(a.requests), a.problem, o.err)
	for _, request := range a.requests {
		message := request.Diagnostic
		if len(message) > 4096 {
			message = message[:4096] + " (truncated)"
		}
		fmt.Fprintf(&out, "%s %s HTTP %d cancelled=%t diagnostic=%q\n", request.Method, request.Path, request.Status, request.Cancelled, message)
		if state := request.State; state != nil {
			fmt.Fprintf(&out, "  engine state: status=%v running=%v exit=%v error=%q\n", state["Status"], state["Running"], state["ExitCode"], state["Error"])
		}
	}
	return out.String()
}
