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
				reviews, publications = 0, 1
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

func TestAllowlistConsentRequiresTerminalAndPreservesPrecedence(t *testing.T) {
	for _, kind := range []string{"yes", "decline", "matching", "matching redirected", "redirected", "json", "fingerprint", "fingerprint terminal", "matching fingerprint", "wrong fingerprint", "bad fingerprint", "unready fingerprint", "state fingerprint"} {
		t.Run(kind, func(t *testing.T) {
			good := "sha256:" + strings.Repeat("a", 64)
			service := &porttest.Service{View: approval.View{Inspection: enginefake.Ready(), Status: "missing", Candidate: approval.Candidate{Fingerprint: good}}}
			service.View.Inspection.Policy.Policy.Version = 2
			service.View.Inspection.Policy.Policy.Network.Mode = "allowlist"
			ui := &porttest.UI{IsTerminal: true, Yes: true}
			var fingerprint *string
			withJSON := false
			want := approval.Failure("")
			reviews, publications := 1, 1
			switch kind {
			case "yes":
			case "decline":
				ui.Yes = false
				want = "approve.declined"
				publications = 0
			case "matching":
				service.View.Status = "matching"
				// Revalidate control state without asking for consent again.
				reviews, publications = 0, 1
			case "matching redirected", "redirected", "json":
				if kind == "matching redirected" {
					service.View.Status = "matching"
				}
				if kind == "json" {
					withJSON = true
				} else {
					ui.IsTerminal = false
				}
				want = "approve.confirmation-required"
				reviews, publications = 0, 0
			default:
				fingerprint = &good
				ui.IsTerminal = kind == "fingerprint terminal"
				want = "approve.terminal-required"
				reviews, publications = 0, 0
				switch kind {
				case "matching fingerprint":
					service.View.Status = "matching"
				case "wrong fingerprint":
					value := "sha256:" + strings.Repeat("b", 64)
					fingerprint = &value
				case "bad fingerprint":
					value := ""
					fingerprint = &value
				case "unready fingerprint":
					service.View.Inspection.Ready = false
					want = ""
				case "state fingerprint":
					service.View.Failure = "state.invalid"
					want = "state.invalid"
				}
			}
			v := Approve(service, ui, fingerprint, withJSON)
			if v.Failure != want || ui.Reviews != reviews || service.Publications != publications {
				t.Fatal(v, ui.Reviews, service.Publications)
			}
			if want == "approve.confirmation-required" || want == "approve.terminal-required" {
				problems := approval.Problems(v)
				if len(problems) != 1 || problems[0].Exit != 1 || !strings.Contains(problems[0].Fix, "itos container approve") || !strings.Contains(problems[0].Fix, "terminal") || strings.Contains(problems[0].Fix, "--fingerprint") {
					t.Fatal(problems)
				}
			}
		})
	}
}
