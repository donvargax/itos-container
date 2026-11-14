package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestCredentialDiagnosticEnvelope(t *testing.T) {
	for _, rule := range []string{"missing", "unavailable", "mount_id", "filesystem", "symlink", "ancestor", "location", "regular", "owner", "permissions", "links", "size", "duplicate"} {
		p := credentialProblem(readiness.CredentialIssue{Rule: "credential." + rule, Index: 1})
		want := 1
		if rule == "missing" || rule == "unavailable" || rule == "mount_id" || rule == "filesystem" {
			want = 3
		}
		if p.Rule != "credential."+rule || p.Exit != want || p.File != "itos-container.yaml" || p.Path == nil || *p.Path != "/credentials/1/source" || p.Message == "" || p.Fix == "" {
			t.Fatal(p)
		}
	}
	// The first written source is index zero: it is located like any other,
	// and only a boundary refusal has no source path.
	if p := credentialProblem(readiness.CredentialIssue{Rule: "credential.missing", Index: 0}); p.Path == nil || *p.Path != "/credentials/0/source" {
		t.Fatalf("first written source lost its location: %#v", p)
	}
	if p := credentialProblem(readiness.CredentialIssue{Rule: "state.location", Index: -1}); p.Path != nil && *p.Path != "" {
		t.Fatalf("boundary refusal located at a source: %#v", p)
	}
	// One rule refuses every filesystem whose mount rows cannot place a file.
	message := credentialProblem(readiness.CredentialIssue{Rule: "credential.filesystem"}).Message
	for _, kind := range []string{"Overlay", "FUSE", "NFS"} {
		if !strings.Contains(message, kind) {
			t.Fatalf("filesystem refusal does not name %s: %q", kind, message)
		}
	}
	for rule, exit := range map[string]int{"state.location": 2, "state.path": 1, "state.unsafe": 1, "state.unavailable": 3, "checkout.unavailable": 3, "checkout.root": 2, "checkout.path": 1, "checkout.changed": 1, "host.unsupported": 1, "unknown": 70} {
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
	if !strings.Contains(errout.String(), "itos-container.yaml/credentials/0/source: Credential source is absent; ") {
		t.Fatalf("plain output does not locate the first written source: %s", errout.String())
	}
}

func TestCredentialCheckoutDiagnosticRetainsSubject(t *testing.T) {
	for _, rule := range []string{"checkout.path", "checkout.root", "checkout.unavailable", "checkout.changed"} {
		var out, errout bytes.Buffer
		result := readiness.CredentialResult{Issues: []readiness.CredentialIssue{{Rule: rule, Index: -1, Subject: "/projects/checkout"}}}
		p := credentialProblem(result.Issues[0])
		if p.Path == nil || *p.Path != "/projects/checkout" {
			t.Fatal(p)
		}
		presentCredentials(&cli.UI{Stdout: &out, Stderr: &errout}, nil, result, false)
		if !strings.Contains(errout.String(), "itos-container: /projects/checkout: ") {
			t.Fatal(errout.String())
		}
	}
}

type credentialEnvelope struct {
	Schema      int
	Scope       string
	OK          bool
	Credentials []credentialGrant
	Problems    []struct {
		Rule string
		Exit int
		Path *string
	}
}

func runCredentialsOnly(t *testing.T, c CLI) (int, credentialEnvelope, string) {
	t.Helper()
	var out, errout bytes.Buffer
	c.CredentialsOnly, c.JSON = true, true
	code := c.Run(&cli.UI{Stdout: &out, Stderr: &errout})
	var got credentialEnvelope
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err, out.String())
	}
	return code, got, out.String()
}

// The scope has two policy readers: only the one bound to the pinned root may
// supply its requests, and the file-only scopes never use that one.
func TestCredentialsOnlyReadsTheRootBoundPolicy(t *testing.T) {
	sources := &porttest.Credentials{}
	working := &porttest.Policies{Result: policy.Result{Policy: &policy.Policy{Version: 3, Manifest: "a.json", Credentials: []policy.Credential{{Kind: "github-token", Source: "/working/token"}}}}}
	rooted := &porttest.Policies{Result: policy.Result{Policy: &policy.Policy{Version: 3, Manifest: "a.json", Credentials: []policy.Credential{{Kind: "claude-token", Source: "/private/token"}}}}, Root: sources}
	code, got, raw := runCredentialsOnly(t, CLI{Policies: working, RootPolicies: rooted, Credentials: sources})
	if code != 0 || !got.OK || got.Scope != "credentials" || len(got.Credentials) != 1 || got.Credentials[0].Source != "/private/token" {
		t.Fatalf("%d %s", code, raw)
	}
	if working.Reads != 0 || rooted.Reads != 1 || len(sources.Calls) != 1 || sources.Calls[0][0].Source != "/private/token" || sources.Pinned {
		t.Fatalf("working reads %d, rooted reads %d, calls %v, pinned %t", working.Reads, rooted.Reads, sources.Calls, sources.Pinned)
	}

	var out, errout bytes.Buffer
	file := CLI{Policies: working, RootPolicies: rooted, Credentials: sources, PolicyOnly: true, JSON: true}
	if code := file.Run(&cli.UI{Stdout: &out, Stderr: &errout}); code != 0 || working.Reads != 1 || rooted.Reads != 1 || !strings.Contains(out.String(), "/working/token") {
		t.Fatalf("policy-only left the working directory: %d %s", code, out.String())
	}
}

func TestCredentialsOnlyRefusedRootReportsNoRequest(t *testing.T) {
	sources := &porttest.Credentials{Refusal: readiness.CredentialResult{Issues: []readiness.CredentialIssue{{Rule: "checkout.path", Index: -1, Subject: "/projects/checkout"}}}}
	rooted := &porttest.Policies{Result: policy.Result{Policy: &policy.Policy{Version: 3, Credentials: []policy.Credential{{Kind: "claude-token", Source: "/private/token"}}}}, Root: sources}
	code, got, raw := runCredentialsOnly(t, CLI{Policies: rooted, RootPolicies: rooted, Credentials: sources})
	if code != 1 || got.OK || got.Schema != 1 || got.Scope != "credentials" || !strings.Contains(raw, `"credentials":[]`) {
		t.Fatalf("%d %s", code, raw)
	}
	if len(got.Problems) != 1 || got.Problems[0].Rule != "doctor.checkout.path" || got.Problems[0].Exit != 1 || got.Problems[0].Path == nil || *got.Problems[0].Path != "/projects/checkout" {
		t.Fatalf("%s", raw)
	}
	if rooted.Reads != 0 || len(sources.Calls) != 0 {
		t.Fatalf("a refused root was read: %d reads, calls %v", rooted.Reads, sources.Calls)
	}
}

func TestCredentialsOnlyPolicyRefusalKeepsThePolicyEnvelope(t *testing.T) {
	sources := &porttest.Credentials{}
	rooted := &porttest.Policies{Result: policy.Result{Issues: []policy.Issue{{Kind: policy.Missing}}}, Root: sources}
	var out, errout bytes.Buffer
	c := CLI{RootPolicies: rooted, Credentials: sources, CredentialsOnly: true, JSON: true}
	if code := c.Run(&cli.UI{Stdout: &out, Stderr: &errout}); code != 2 || !strings.Contains(out.String(), `"scope":"policy"`) || !strings.Contains(out.String(), `"rule":"policy.missing"`) {
		t.Fatalf("%d %s", code, out.String())
	}
	if len(sources.Calls) != 0 || sources.Pinned {
		t.Fatalf("policy refusal reached sources or kept the root: %v %t", sources.Calls, sources.Pinned)
	}
}

// Each pair refuses as usage before any reader, pin or source is touched.
func TestCredentialsOnlyCombinesWithNoOtherScope(t *testing.T) {
	for _, tc := range []struct{ policyOnly, manifestOnly bool }{{true, false}, {false, true}, {true, true}} {
		var out, errout bytes.Buffer
		sources := &porttest.Credentials{}
		working, rooted, manifests := &porttest.Policies{}, &porttest.Policies{Root: sources}, &porttest.Manifests{}
		c := CLI{Policies: working, RootPolicies: rooted, Manifests: manifests, Credentials: sources, CredentialsOnly: true, PolicyOnly: tc.policyOnly, ManifestOnly: tc.manifestOnly, JSON: true}
		if code := c.Run(&cli.UI{Stdout: &out, Stderr: &errout}); code != 2 || !strings.Contains(out.String(), `"rule":"usage"`) || !strings.Contains(errout.String(), "cannot be combined") {
			t.Fatalf("%+v: %d %s %s", tc, code, out.String(), errout.String())
		}
		if working.Reads != 0 || rooted.Reads != 0 || len(manifests.Paths) != 0 || len(sources.Events) != 0 {
			t.Fatalf("%+v touched a port: %d %d %v %v", tc, working.Reads, rooted.Reads, manifests.Paths, sources.Events)
		}
	}
}
