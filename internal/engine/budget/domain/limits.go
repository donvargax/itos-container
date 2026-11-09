package domain

import "time"

// These fixed product limits have no runtime configuration. The native scenario
// harness substitutes this file at build time in its disposable test binary.
const (
	CallDuration    = 10 * time.Second
	StartupDuration = 120 * time.Second
	GraceDuration   = 10 * time.Second
)
