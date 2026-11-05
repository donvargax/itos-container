// Package porttest provides read-only run approval fixtures.
package porttest

import approval "github.com/donvargax/itos-container/internal/engine/approval/port"

// Approval records inspection/selection without any publication capability.
type Approval struct {
	View                    approval.View
	Failure                 approval.Failure
	Inspections, Selections int
}

func (a *Approval) Inspect() approval.View { a.Inspections++; return a.View }
func (a *Approval) Freeze(v approval.View) (approval.Frozen, approval.Failure) {
	a.Selections++
	if a.Failure != "" {
		return approval.Frozen{}, a.Failure
	}
	return approval.Frozen{Project: v.Candidate.Project, Generation: v.Record.Generation, Fingerprint: v.Candidate.Fingerprint, Settings: v.Candidate.Canonical}, ""
}
