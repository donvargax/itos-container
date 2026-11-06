package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/donvargax/itos-container/internal/cli"
)

func TestRunOptionsStayInTheirCommand(t *testing.T) {
	for _, tc := range []struct{ args, want []string }{
		{nil, nil},
		{[]string{"--json"}, []string{"--json"}},
		{[]string{"run"}, []string{"run"}},
		{[]string{"--json", "run", "--", "git", "--no-json"}, []string{"run", "--json", "--", "git", "--no-json"}},
		{[]string{"--no-json", "run", "--", "git"}, []string{"run", "--no-json", "--", "git"}},
		{[]string{"--json", "--no-json", "run"}, []string{"run", "--json", "--no-json"}},
		{[]string{"--json=true", "status"}, []string{"status", "--json=true"}},
		{[]string{"--json=false", "stop"}, []string{"stop", "--json=false"}},
		{[]string{"--json=true", "--no-json", "status"}, []string{"status", "--json=true", "--no-json"}},
		{[]string{"--json=false", "run", "--", "git", "--json=true"}, []string{"run", "--json=false", "--", "git", "--json=true"}},
		{[]string{"--json=true", "--", "status"}, []string{"--json=true", "--", "status"}},
		{[]string{"count", "run", "--json"}, []string{"count", "run", "--json"}},
		{[]string{"--json", "doctor"}, []string{"--json", "doctor"}},
		{[]string{"--", "run", "--json"}, []string{"--", "run", "--json"}},
		{[]string{"--bogus", "run"}, []string{"--bogus", "run"}},
	} {
		before := slices.Clone(tc.args)
		if got := runOptions(tc.args); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("runOptions(%q) = %q, want %q", tc.args, got, tc.want)
		}
		if !reflect.DeepEqual(tc.args, before) {
			t.Fatal("modified caller argv")
		}
	}
}

// This checks only parsing, not eventual engine delivery of the argv. The latter
// remains a separate real-engine scenario in the offline-run implementation.
func TestKongKeepsTheFirstRunSeparatorAndLiteralWords(t *testing.T) {
	var c commandLine
	parser, err := kong.New(&c, cli.Flags()...)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--", "git", "", "two words", "a,b", "\t\n", "λ", "--", "--help", "--no-json", "$HOME;*"}
	args := append([]string{"--json", "run"}, want...)
	if _, err := parser.Parse(runOptions(args)); err != nil {
		t.Fatal(err)
	}
	if !c.Run.JSON || !reflect.DeepEqual(c.Run.Args, want) {
		t.Fatalf("JSON = %v, argv = %q", c.Run.JSON, c.Run.Args)
	}
}

func TestRunAssemblyAndHelp(t *testing.T) {
	// The empty checkout fails before approval lookup. Effective host identity
	// is checked even earlier, including when this test runs as root or on Windows.
	preflightCode, preflightRule := 2, "policy.missing"
	switch {
	case os.Geteuid() < 0 || os.Getegid() < 0:
		preflightCode, preflightRule = 3, "approval.identity.unavailable"
	case os.Geteuid() == 0:
		preflightCode, preflightRule = 1, "approval.identity.root"
	}
	for _, tc := range []struct {
		name, inherited string
		args            []string
		code            int
		json            bool
	}{
		{"wrapper", "0", []string{"--json", "run", "--", "git"}, 2, true},
		{"inherited", "1", []string{"run", "--", "git", "--no-json"}, 2, true},
		{"workload JSON flag", "0", []string{"run", "--", "--json"}, preflightCode, false},
		{"missing", "", []string{"run"}, 2, false},
		{"help", "", []string{"run", "--help"}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ITOS_JSON", tc.inherited)
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("ITOS_ROOT", root)
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			var stdout, stderr strings.Builder
			code := run(tc.args, strings.NewReader(""), &stdout, &stderr)
			if code != tc.code {
				t.Fatalf("code = %d, want %d; stderr %s", code, tc.code, stderr.String())
			}
			if tc.name == "workload JSON flag" && !strings.Contains(stderr.String(), preflightRule) {
				t.Fatalf("stderr = %q, want preflight rule %q", stderr.String(), preflightRule)
			}
			if tc.json {
				if !strings.Contains(stdout.String(), `"rule":"run.json-unsupported"`) {
					t.Fatalf("stdout = %q", stdout.String())
				}
			} else if tc.code == 0 {
				if !strings.Contains(stdout.String(), "approved offline worker") || stderr.Len() != 0 {
					t.Fatalf("help stdout %q, stderr %q", stdout.String(), stderr.String())
				}
			} else if stdout.Len() != 0 {
				t.Fatalf("unexpected stdout %q", stdout.String())
			}
		})
	}
}
