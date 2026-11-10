// Package domain owns explicit approval consent and no-op decisions.
package domain

import (
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	engine "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/slices/approve/port"
)

// Approve accepts interactive consent only after showing the complete review.
func Approve(service port.Service, ui engine.Reviewer, fingerprint *string, json bool) engine.View {
	view := service.Inspect()
	if !view.Inspection.Ready || view.Failure != "" {
		return view
	}
	if fingerprint != nil && view.RequiresTerminal() {
		view.Failure = "approve.terminal-required"
		return view
	}
	if fingerprint != nil {
		if !approval.ValidFingerprint(*fingerprint) {
			view.Failure = "approve.fingerprint-format"
			return view
		}
		if *fingerprint != view.Candidate.Fingerprint {
			view.Failure = "approve.fingerprint"
			return view
		}
	} else if json || !ui.Terminal() {
		view.Failure = "approve.confirmation-required"
		return view
	}
	if view.Status == "matching" {
		return service.Approve(view)
	}
	if fingerprint == nil && !ui.Review(view) {
		view.Failure = "approve.declined"
		return view
	}
	return service.Approve(view)
}
