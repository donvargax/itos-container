package port_test

import (
	"reflect"
	"testing"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestProblemsEmpty(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result port.Result
	}{
		{"absent inspections", port.Result{}},
		{"inspections without issues", port.Result{Policy: &policy.Result{}, Manifest: &manifest.Result{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := port.Problems(tc.result); !reflect.DeepEqual(got, []port.Problem{}) {
				t.Fatalf("problems = %#v; want a non-nil empty list", got)
			}
		})
	}
}

func TestHardlinkDiagnosticNamesOnlyTheCheckout(t *testing.T) {
	root := "/checkout"
	got := port.Problems(port.Result{Issues: []port.Issue{{Kind: port.CheckoutHardlink, Subject: root}}})
	want := []port.Problem{{Rule: "doctor.checkout.hardlink", Exit: 1, Path: &root,
		Message: "Checkout contains hard links that share file inodes",
		Fix:     "prepare an independent checkout without shared file inodes"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hard-link prerequisite = %#v, want %#v", got, want)
	}
}

func TestProblemsPreservesDiagnostics(t *testing.T) {
	result := port.Result{
		Policy: &policy.Result{Issues: []policy.Issue{
			{Kind: policy.Unreadable},
			{Kind: policy.UnknownKey, Path: []string{"extra/~field"}, Version: 1},
		}},
		Manifest: &manifest.Result{Path: "worker.json", Image: "image-subject", Issues: []manifest.Issue{
			{Kind: manifest.Unsupported, Path: []string{"initializeCommand"}},
			{Kind: manifest.MetadataFailure, Path: []string{"0", "remoteUser"}},
		}},
		Issues: []port.Issue{
			{Kind: port.CheckoutUnavailable, Subject: "/checkout"},
			{Kind: port.EngineSelection, Subject: "DOCKER_HOST"},
		},
	}
	root, policyPath := "", "/extra~1~0field"
	manifestPath, metadataPath, checkoutPath := "/initializeCommand", "/0/remoteUser", "/checkout"
	want := []port.Problem{
		{Rule: "policy.unreadable", Exit: 3, File: "itos-container.yaml", Path: &root,
			Message: "Policy file cannot be read",
			Fix:     "provide a readable regular policy file of at most 65536 bytes, not a symbolic link"},
		{Rule: "policy.unknown_key", Exit: 2, File: "itos-container.yaml", Path: &policyPath,
			Message: "Field is not supported", Fix: "remove fields absent from the version 1 schema"},
		{Rule: "manifest.unsupported", Exit: 2, File: "worker.json", Path: &manifestPath,
			Message: "Field is not supported",
			Fix:     "remove fields outside the restricted worker profile, including lifecycle hooks"},
		{Rule: "manifest.metadata", Exit: 2, Image: "image-subject", Label: "devcontainer.metadata", Path: &metadataPath,
			Message: "Image metadata is outside the descriptive-only profile",
			Fix:     "supply strict JSON within inspection limits containing objects with only valid names and unused editor customizations"},
		{Rule: "doctor.checkout.unavailable", Exit: 3, Path: &checkoutPath,
			Message: "Checkout root is unavailable", Fix: "provide a readable checkout directory"},
		{Rule: "doctor.engine.selection", Exit: 2, Field: "DOCKER_HOST",
			Message: "Engine selection is unsupported",
			Fix:     "use only DOCKER_HOST=unix:///absolute/socket without context, TLS or API overrides"},
	}
	if got := port.Problems(result); !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %#v; want %#v", got, want)
	}
}

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
		{"proxy executable", port.Issue{Kind: port.EgressExecutable, Subject: "executable"}, port.Problem{
			Rule: "doctor.egress-executable", Exit: 3, Field: "executable",
			Message: "This program cannot be delivered to the egress proxy",
			Fix:     "use the installed static Linux amd64 release of itos-container, from a path outside the checkout",
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
