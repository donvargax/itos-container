package domain_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/domain"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func TestHomeHarnessRequests(t *testing.T) {
	opencode := text("opencode")
	for _, version := range []int{1, 2, 3} {
		for name, tc := range map[string]struct {
			harnesses port.Value
			kind      port.Failure
			path      []string
		}{
			"opencode":  {array(opencode), port.None, nil},
			"null":      {port.Value{}, port.Type, []string{"home", "harnesses"}},
			"string":    {opencode, port.Type, []string{"home", "harnesses"}},
			"object":    {object(nil), port.Type, []string{"home", "harnesses"}},
			"empty":     {array(), port.HomeHarness, []string{"home", "harnesses"}},
			"codex":     {array(text("codex")), port.HomeHarness, []string{"home", "harnesses", "0"}},
			"spelling":  {array(text("OpenCode")), port.HomeHarness, []string{"home", "harnesses", "0"}},
			"duplicate": {array(opencode, opencode), port.HomeHarness, []string{"home", "harnesses", "1"}},
			"number":    {array(number("7", "0")), port.Type, []string{"home", "harnesses", "0"}},
		} {
			root := minimal()
			root.Fields["version"] = number(strconv.Itoa(version), "0")
			root.Fields["home"] = object(map[string]port.Value{"storage": text("volume"), "harnesses": tc.harnesses})
			result := domain.Inspect(&porttest.Source{Value: root}, nil)
			if tc.kind == port.None {
				if result.Policy == nil || !reflect.DeepEqual(result.Policy.Home, &port.Home{Storage: "volume", Harnesses: []string{"opencode"}}) || len(result.Issues) != 0 {
					t.Fatal(name, result)
				}
				continue
			}
			if result.Policy != nil || !reflect.DeepEqual(result.Issues, []port.Issue{{Kind: tc.kind, Path: tc.path, Version: version}}) {
				t.Fatal(name, result)
			}
		}
		// Omitting harnesses keeps the home output as it was: no harness list.
		root := minimal()
		root.Fields["version"] = number(strconv.Itoa(version), "0")
		root.Fields["home"] = object(map[string]port.Value{"storage": text("volume")})
		result := domain.Inspect(&porttest.Source{Value: root}, nil)
		if result.Policy == nil || !reflect.DeepEqual(result.Policy.Home, &port.Home{Storage: "volume"}) || result.Policy.Home.Harnesses != nil || len(result.Issues) != 0 {
			t.Fatal("omitted", result)
		}
	}
}

func TestHomeHarnessDiagnostic(t *testing.T) {
	d := port.Diagnostic(port.Issue{Kind: port.HomeHarness, Path: []string{"home", "harnesses", "0"}, Version: 2})
	if d.Rule != "policy.home_harness" || d.Exit != 2 || d.Message == "" || d.Fix != "use home.harnesses [opencode], or omit harnesses" {
		t.Fatal(d)
	}
}
