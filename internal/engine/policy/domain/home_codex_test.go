package domain_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/domain"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func TestHomeCodexHarnessRequests(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		for name, tc := range map[string]struct {
			home map[string]port.Value
			want *port.Home
			kind port.Failure
			path []string
		}{
			"codex":      {map[string]port.Value{"storage": text("volume"), "harness": text("codex")}, &port.Home{Storage: "volume", Harness: "codex"}, port.None, nil},
			"both":       {map[string]port.Value{"storage": text("volume"), "harness": text("codex"), "harnesses": array(text("opencode"))}, &port.Home{Storage: "volume", Harness: "codex", Harnesses: []string{"opencode"}}, port.None, nil},
			"null":       {map[string]port.Value{"storage": text("volume"), "harness": {}}, nil, port.Type, []string{"home", "harness"}},
			"number":     {map[string]port.Value{"storage": text("volume"), "harness": number("7", "0")}, nil, port.Type, []string{"home", "harness"}},
			"list":       {map[string]port.Value{"storage": text("volume"), "harness": array(text("codex"))}, nil, port.Type, []string{"home", "harness"}},
			"object":     {map[string]port.Value{"storage": text("volume"), "harness": object(nil)}, nil, port.Type, []string{"home", "harness"}},
			"empty":      {map[string]port.Value{"storage": text("volume"), "harness": text("")}, nil, port.HomeHarness, []string{"home", "harness"}},
			"case":       {map[string]port.Value{"storage": text("volume"), "harness": text("Codex")}, nil, port.HomeHarness, []string{"home", "harness"}},
			"space":      {map[string]port.Value{"storage": text("volume"), "harness": text(" codex")}, nil, port.HomeHarness, []string{"home", "harness"}},
			"other":      {map[string]port.Value{"storage": text("volume"), "harness": text("opencode")}, nil, port.HomeHarness, []string{"home", "harness"}},
			"seed name":  {map[string]port.Value{"storage": text("volume"), "harness": text("codex-v1")}, nil, port.HomeHarness, []string{"home", "harness"}},
			"no storage": {map[string]port.Value{"harness": text("codex")}, nil, port.Required, []string{"home", "storage"}},
			"tmpfs":      {map[string]port.Value{"storage": text("tmpfs"), "harness": text("codex")}, nil, port.HomeStorage, []string{"home", "storage"}},
			"list codex": {map[string]port.Value{"storage": text("volume"), "harnesses": array(text("codex"))}, nil, port.HomeHarness, []string{"home", "harnesses", "0"}},
		} {
			root := minimal()
			root.Fields["version"] = number(strconv.Itoa(version), "0")
			root.Fields["home"] = object(tc.home)
			result := domain.Inspect(&porttest.Source{Value: root}, nil)
			if tc.kind == port.None {
				if result.Policy == nil || !reflect.DeepEqual(result.Policy.Home, tc.want) || len(result.Issues) != 0 {
					t.Fatal(name, result)
				}
				continue
			}
			if result.Policy != nil || !reflect.DeepEqual(result.Issues, []port.Issue{{Kind: tc.kind, Path: tc.path, Version: version}}) {
				t.Fatal(name, result)
			}
		}
	}
}

func TestHomeCodexHarnessDiagnostics(t *testing.T) {
	d := port.Diagnostic(port.Issue{Kind: port.HomeHarness, Path: []string{"home", "harness"}, Version: 1})
	if d.Rule != "policy.home_harness" || d.Exit != 2 || d.Path != "/home/harness" || d.Message != "Home harness is unsupported" || d.Fix != "use home.harness codex, or omit harness" {
		t.Fatal(d)
	}
	d = port.Diagnostic(port.Issue{Kind: port.HomeHarnessGated, Path: []string{"home", "harness"}, Version: 1})
	if d.Rule != "policy.home_harness" || d.Exit != 2 || d.Path != "/home/harness" || d.Message != "Home harness is not accepted by this command yet" || d.Fix != "omit home.harness to run, or review what it requests with doctor and approve" {
		t.Fatal(d)
	}
}

func TestReservedEnvironmentFollowsTheCodexSelection(t *testing.T) {
	for name, tc := range map[string]struct {
		policy port.Policy
		want   []string
	}{
		"no home":   {port.Policy{}, nil},
		"volume":    {port.Policy{Home: &port.Home{Storage: "volume"}}, nil},
		"opencode":  {port.Policy{Home: &port.Home{Storage: "volume", Harnesses: []string{"opencode"}}}, nil},
		"codex":     {port.Policy{Home: &port.Home{Storage: "volume", Harness: "codex"}}, []string{"CODEX_HOME"}},
		"codex too": {port.Policy{Home: &port.Home{Storage: "volume", Harness: "codex", Harnesses: []string{"opencode"}}}, []string{"CODEX_HOME"}},
	} {
		if got := tc.policy.ReservedEnvironment(); !reflect.DeepEqual(got, tc.want) {
			t.Fatal(name, got)
		}
	}
}
