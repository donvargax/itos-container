package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestCredentialDiagnosticEnvelope(t *testing.T) {
	for _, rule := range []string{"missing", "unavailable", "symlink", "ancestor", "location", "regular", "owner", "permissions", "links", "size", "duplicate"} {
		p := credentialProblem(readiness.CredentialIssue{Rule: "credential." + rule, Index: 1})
		want := 1
		if rule == "missing" || rule == "unavailable" {
			want = 3
		}
		if p.Rule != "credential."+rule || p.Exit != want || p.File != "itos-container.yaml" || p.Path == nil || *p.Path != "/credentials/1/source" || p.Message == "" || p.Fix == "" {
			t.Fatal(p)
		}
	}
	for rule, exit := range map[string]int{"state.location": 2, "state.path": 1, "state.unsafe": 1, "state.unavailable": 3, "checkout.unavailable": 3, "checkout.root": 2, "host.unsupported": 1, "unknown": 70} {
		p := credentialProblem(readiness.CredentialIssue{Rule: rule, Index: -1})
		want := rule
		if strings.HasPrefix(rule, "state.") {
			want = "approval." + rule
		} else if rule != "unknown" {
			want = "doctor." + rule
		}
		if p.Rule != want || p.Exit != exit || p.Path != nil && *p.Path != "" || p.Message == "" || p.Fix == "" {
			t.Fatal(p)
		}
	}
}

func TestCredentialPresentationRetainsSortedRequestsOnFailure(t *testing.T) {
	requests := []policy.Credential{{Kind: "github-token", Source: "/a\u0080"}, {Kind: "claude-token", Source: "/b"}}
	result := readiness.CredentialResult{Issues: []readiness.CredentialIssue{{Rule: "credential.owner", Index: 1}, {Rule: "credential.missing", Index: 0}}}
	var out, errout bytes.Buffer
	ui := &cli.UI{Stdout: &out, Stderr: &errout}
	if code := presentCredentials(ui, requests, result, true); code != 3 {
		t.Fatal(code)
	}
	var got struct {
		Credentials []credentialGrant
		Problems    []policy.Problem
		Scope       string
		OK          bool
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OK || got.Scope != "credentials" || len(got.Credentials) != 2 || got.Credentials[0].Kind != "claude-token" || got.Credentials[1].Source != "/a\u0080" || len(got.Problems) != 2 {
		t.Fatal(got)
	}
	out.Reset()
	errout.Reset()
	if code := presentCredentials(ui, requests, result, false); code != 3 || !strings.Contains(out.String(), `/a\u0080`) || !strings.Contains(out.String(), "Credential contents, manifest, engine and approval were not checked") {
		t.Fatalf("%d %s", code, out.String())
	}
}
