package port_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestDiagnosticRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		issue port.Issue
		want  port.Problem
	}{
		{"unsupported host", port.Issue{Kind: port.HostUnsupported, Subject: "host"}, port.Problem{
			Rule: "doctor.host.unsupported", Exit: 1, Field: "host",
			Message: "Host is outside the inspection target", Fix: "use Linux amd64 with native Docker",
		}},
		{"image user", port.Issue{Kind: port.ImageUser, Subject: "image-subject"}, port.Problem{
			Rule: "doctor.image.user", Exit: 1, Image: "image-subject",
			Message: "Image user does not match the manifest's numeric identity request",
			Fix:     "prepare an image with a positive numeric UID matching each specified manifest user",
		}},
		{"unknown failure", port.Issue{Kind: port.Failure(255), Subject: "inspection"}, port.Problem{
			Rule: "doctor.internal", Exit: 70, Field: "inspection",
			Message: "Unknown inspection failure", Fix: "report this as a bug",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := port.Diagnostic(tc.issue); got != tc.want {
				t.Fatalf("inspection diagnostic = %#v; want %#v", got, tc.want)
			}
		})
	}
}
