package port_test

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func TestUnknownDiagnostic(t *testing.T) {
	got := port.Diagnostic(port.Issue{Kind: port.Failure(255), Path: []string{"a/b~c", ""}})
	want := port.Problem{
		Rule: "policy.internal", Exit: 70, File: "itos-container.yaml", Path: "/a~1b~0c/",
		Message: "Unknown policy failure", Fix: "report this as a bug",
	}
	if got != want {
		t.Fatalf("unknown policy diagnostic = %#v; want %#v", got, want)
	}
}

// A fix points at the schema the file was judged against, never at another one.
func TestFixNamesTheVersionRead(t *testing.T) {
	for _, tc := range []struct {
		issue port.Issue
		rule  string
		fix   string
	}{
		{port.Issue{Kind: port.Type, Version: 1}, "policy.type", "use the field type defined by policy version 1"},
		{port.Issue{Kind: port.Type, Version: 2}, "policy.type", "use the field type defined by policy version 2"},
		{port.Issue{Kind: port.Type}, "policy.type", "use the field type defined by policy version 1, 2 or 3"},
		{port.Issue{Kind: port.Version}, "policy.version", "use policy version 1, 2 or 3"},
		{port.Issue{Kind: port.Version, Version: 2}, "policy.version", "use policy version 1, or read what a version 2 file requests with doctor --policy-only"},
		{port.Issue{Kind: port.UnknownKey, Version: 1}, "policy.unknown_key", "remove fields absent from the version 1 schema"},
		{port.Issue{Kind: port.UnknownKey, Version: 2}, "policy.unknown_key", "remove fields absent from the version 2 schema"},
		{port.Issue{Kind: port.NetworkMode, Version: 1}, "policy.network_mode", "use offline mode"},
		{port.Issue{Kind: port.NetworkMode, Version: 2}, "policy.network_mode", "use offline or allowlist mode"},
	} {
		got := port.Diagnostic(tc.issue)
		if got.Rule != tc.rule || got.Fix != tc.fix || got.Exit != 2 || got.Message == "" {
			t.Errorf("Diagnostic(%#v) = %#v; want rule %q, fix %q, exit 2", tc.issue, got, tc.rule, tc.fix)
		}
	}
}

func TestAllowlistDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		kind          port.Failure
		rule, message string
	}{
		{port.HostName, "policy.host_name", "Host name is not a full DNS name"},
		{port.HostDuplicate, "policy.host_duplicate", "Host name repeats an earlier entry"},
		{port.AllowEmpty, "policy.allow_empty", "Allowlist is empty"},
		{port.AllowOffline, "policy.allow_offline", "Allowlist needs allowlist mode"},
	} {
		got := port.Diagnostic(port.Issue{Kind: tc.kind, Path: []string{"network", "allow", "3"}, Version: 2})
		if got.Rule != tc.rule || got.Message != tc.message || got.Exit != 2 || got.Fix == "" || got.File != "itos-container.yaml" || got.Path != "/network/allow/3" {
			t.Errorf("Diagnostic(%v) = %#v", tc.kind, got)
		}
		if strings.Contains(got.Fix, "version 1") {
			t.Errorf("a version 2 fix names version 1: %q", got.Fix)
		}
	}
}
