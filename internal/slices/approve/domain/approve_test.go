package domain

import (
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	enginefake "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/approve/port/porttest"
	"strings"
	"testing"
)

func TestConsentAndPrecedence(t *testing.T) {
	for _, kind := range []string{"yes", "decline", "missing terminal", "json", "matching", "matching redirected", "fingerprint", "wrong fingerprint", "bad format", "prerequisite", "state"} {
		t.Run(kind, func(t *testing.T) {
			good := "sha256:" + strings.Repeat("a", 64)
			service := &porttest.Service{View: approval.View{Inspection: enginefake.Ready(), Status: "missing", Candidate: approval.Candidate{Fingerprint: good}}}
			ui := &porttest.UI{IsTerminal: true, Yes: true}
			var fingerprint *string
			json := false
			want := approval.Failure("")
			reviews, publications := 1, 1
			switch kind {
			case "decline":
				ui.Yes = false
				want = "approve.declined"
				publications = 0
			case "missing terminal", "matching redirected":
				ui.IsTerminal = false
				want = "approve.confirmation-required"
				reviews, publications = 0, 0
				if kind == "matching redirected" {
					service.View.Status = "matching"
				}
			case "json":
				json = true
				want = "approve.confirmation-required"
				reviews, publications = 0, 0
			case "matching":
				service.View.Status = "matching"
				reviews, publications = 0, 0
			case "fingerprint":
				fingerprint = &good
				ui.IsTerminal = false
				reviews = 0
			case "wrong fingerprint":
				value := "sha256:" + strings.Repeat("b", 64)
				fingerprint = &value
				want = "approve.fingerprint"
				reviews, publications = 0, 0
			case "bad format":
				value := ""
				fingerprint = &value
				want = "approve.fingerprint-format"
				reviews, publications = 0, 0
			case "prerequisite":
				service.View.Inspection.Ready = false
				reviews, publications = 0, 0
			case "state":
				service.View.Failure = "state.invalid"
				want = "state.invalid"
				reviews, publications = 0, 0
			}
			result := Approve(service, ui, fingerprint, json)
			if result.Failure != want || ui.Reviews != reviews || service.Publications != publications {
				t.Fatalf("result %#v reviews=%d publications=%d", result, ui.Reviews, service.Publications)
			}
		})
	}
}
