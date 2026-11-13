package domain

import "github.com/donvargax/itos-container/internal/engine/readiness/port"

// ProxyExecutable judges one observation of the controller's own program for
// delivery to the egress proxy, with the checkout root as a value. Slice-27's
// step checkpoint binds only this shape: until its behaviour is built every
// observation is refused, so nothing can be delivered by mistake.
func ProxyExecutable(port.Executable, port.Failure, string) port.Failure {
	return port.EgressExecutable
}
