package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	policyport "github.com/donvargax/itos-container/internal/engine/policy/port"
	readinessport "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestCredentialInvalidStateLocation(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("credential observation requires Linux amd64")
	}
	root := t.TempDir()
	t.Chdir(root)
	policy := "version: 3\nmanifest: a.json\ncredentials: [{kind: claude-token, source: /synthetic/token}]\n"
	if err := os.WriteFile(filepath.Join(root, "itos-container.yaml"), []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"relative", ""} {
		t.Run("state="+state, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", state)
			t.Setenv("HOME", "")
			// Every absolute spelling must get past checkout validation and
			// report the invalid state, without requiring native metadata traversal.
			for _, checkout := range []string{root, root + "/", strings.ReplaceAll(root, "/", "//"), root + "/.", root + "/unused/.."} {
				t.Setenv("ITOS_ROOT", checkout)
				var out, errout bytes.Buffer
				code := run([]string{"doctor", "--credentials-only", "--json"}, strings.NewReader(""), &out, &errout)
				if code != 2 || !strings.Contains(out.String(), `"rule":"approval.state.location"`) {
					t.Fatalf("invalid state location with checkout %q: exit %d, stdout %s, stderr %s", checkout, code, out.String(), errout.String())
				}
			}
		})
	}
}

func credentialPolicyFile(t *testing.T, dir, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "itos-container.yaml"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

// The policy and the checkout boundary must name one tree. Neither policy
// requests a source, so the same answers hold on every Linux architecture.
func TestCredentialPolicyComesFromTheEffectiveRoot(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the shared checkout pin refuses the symbolic links in other platforms' temporary directories")
	}
	root, working := t.TempDir(), t.TempDir()
	t.Chdir(working)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("ITOS_ROOT", root)
	inspect := func() (int, string) {
		var out, errout bytes.Buffer
		code := run([]string{"doctor", "--credentials-only", "--json"}, strings.NewReader(""), &out, &errout)
		return code, out.String()
	}
	credentialPolicyFile(t, working, "version: 3\nmanifest: working.json\n")
	if code, out := inspect(); code != 2 || !strings.Contains(out, `"rule":"policy.missing"`) {
		t.Fatalf("a root without a policy borrowed the working directory's: exit %d, %s", code, out)
	}
	credentialPolicyFile(t, root, "version: 3\nmanifest: root.json\n")
	credentialPolicyFile(t, working, "version: 3\nmanifest: [\n")
	if code, out := inspect(); code != 0 || !strings.Contains(out, `"scope":"credentials","ok":true`) {
		t.Fatalf("the root's valid policy was not the one read: exit %d, %s", code, out)
	}
	// Without ITOS_ROOT the working directory is the effective root.
	t.Setenv("ITOS_ROOT", "")
	if code, out := inspect(); code != 2 || !strings.Contains(out, `"rule":"policy.syntax"`) {
		t.Fatalf("the working directory was not the effective root: exit %d, %s", code, out)
	}
	// File-only inspection keeps reading the working directory.
	t.Setenv("ITOS_ROOT", root)
	var out, errout bytes.Buffer
	if code := run([]string{"doctor", "--policy-only", "--json"}, strings.NewReader(""), &out, &errout); code != 2 || !strings.Contains(out.String(), `"rule":"policy.syntax"`) {
		t.Fatalf("policy-only left the working directory: exit %d, %s", code, out.String())
	}
}

// The assembled reader has no location of its own: outside the pin it reads
// nothing, even from a working directory that holds a valid policy.
func TestCredentialAssemblyReadsPolicyOnlyThroughThePin(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the shared checkout pin refuses the symbolic links in other platforms' temporary directories")
	}
	root, working := t.TempDir(), t.TempDir()
	t.Chdir(working)
	credentialPolicyFile(t, root, "version: 3\nmanifest: root.json\n")
	credentialPolicyFile(t, working, "version: 3\nmanifest: working.json\n")
	policies, sources := credentialInspection(readinessport.CredentialInput{Root: root})
	unreadable := func(when string) {
		t.Helper()
		if got := policies.Inspect(); got.Policy != nil || len(got.Issues) != 1 || got.Issues[0].Kind != policyport.Unreadable {
			t.Fatalf("read %s the pin: %#v", when, got)
		}
	}
	unreadable("before")
	if refusal := sources.Pin(); len(refusal.Issues) != 0 {
		t.Fatal(refusal)
	}
	if got := policies.Inspect(); got.Policy == nil || got.Policy.Manifest != "root.json" {
		t.Fatalf("pinned root's policy: %#v", got)
	}
	sources.Close()
	unreadable("after")

	_, absent := credentialInspection(readinessport.CredentialInput{Root: filepath.Join(root, "absent")})
	want := readinessport.CredentialIssue{Rule: "checkout.unavailable", Index: -1, Subject: filepath.Join(root, "absent")}
	if refusal := absent.Pin(); len(refusal.Issues) != 1 || refusal.Issues[0] != want {
		t.Fatalf("absent root: %#v", refusal)
	}
}
