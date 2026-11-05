// Package domain owns approval resolution and selection decisions.
package domain

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Resolve will bind a validated inspection to the effective host identity.
func Resolve(_ readiness.Result, _, _ string, _ port.Codec) (port.Candidate, port.Failure) {
	return port.Candidate{}, "unimplemented"
}

// Publish will preserve the reviewed generation during consented publication.
func Publish(_ port.Store, _ port.Candidate, _ string) (port.Record, port.Failure) {
	return port.Record{}, "unimplemented"
}

// Select will select only the exact current approval as independent frozen input.
func Select(_ port.Store, _ port.Candidate, _ string) (port.Frozen, port.Failure) {
	return port.Frozen{}, "unimplemented"
}
