package command_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/slices/run/command"
	"github.com/donvargax/itos-container/internal/slices/run/domain"
)

func TestRunReportsRefusals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request command.CLI
		code    int
		message string
		json    bool
	}{
		{"no command", command.CLI{}, 2, "run requires a nonempty command after --", false},
		{"no separator", command.CLI{Args: []string{"git", "status"}}, 2, "itos container run -- <command>", false},
		{"empty executable", command.CLI{Args: []string{"--", "", "git"}}, 2, "run requires a nonempty command after --", false},
		{"no startup", command.CLI{Args: []string{"--", "git", "--json"}}, 3, "doctor --policy-only", false},
		{"wrapper JSON", command.CLI{JSON: true, Args: []string{"--", "git", "--no-json"}}, 2, "run does not support wrapper JSON", true},
		{"inherited JSON", command.CLI{InheritedJSON: true}, 2, "run does not support wrapper JSON", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			ui := &cli.UI{Classify: command.Classify, Stdout: &stdout, Stderr: &stderr}
			if got := tc.request.Run(ui); got != tc.code {
				t.Fatalf("code = %d, want %d", got, tc.code)
			}
			if !strings.Contains(stderr.String(), tc.message) || !strings.HasPrefix(stderr.String(), "itos-container:") {
				t.Fatalf("stderr = %q", stderr.String())
			}
			if !tc.json {
				if stdout.Len() != 0 {
					t.Fatalf("unexpected stdout: %q", stdout.String())
				}
				return
			}
			var result struct {
				Schema   int           `json:"schema"`
				OK       bool          `json:"ok"`
				Problems []cli.Problem `json:"problems"`
			}
			if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
				t.Fatal(err)
			}
			if result.Schema != 1 || result.OK || len(result.Problems) != 1 {
				t.Fatalf("incorrect refusal: %s", stdout.String())
			}
			p := result.Problems[0]
			if p.Rule != "run.json-unsupported" || p.Exit != tc.code ||
				!strings.Contains(p.Fix, "--json") || !strings.Contains(p.Fix, "ITOS_JSON=1") {
				t.Fatalf("incorrect problem: %+v", p)
			}
		})
	}
}

func TestClassifierHandlesWrappedAndUnknownFailures(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
		rule string
	}{
		{domain.ErrJSONUnsupported, 2, "run.json-unsupported"},
		{domain.ErrCommandRequired, 2, "run.command-required"},
		{domain.ErrStartupUnavailable, 3, "run.startup-unavailable"},
	} {
		code, problems := command.Classify(fmt.Errorf("context: %w", tc.err))
		if code != tc.code || len(problems) != 1 {
			t.Fatalf("classification = %d, %+v", code, problems)
		}
		p := problems[0]
		if p.Rule != tc.rule || p.Exit != code || p.Message == "" || p.Fix == "" {
			t.Fatalf("incorrect problem: %+v", p)
		}
	}
	if code, problems := command.Classify(errors.New("unexpected failure")); code != 0 || problems != nil {
		t.Fatalf("unknown failure was classified: %d, %+v", code, problems)
	}
}

func TestHelpExplainsInterimScope(t *testing.T) {
	help := (command.CLI{}).Help()
	for _, text := range []string{"itos container run -- <command>", "ITOS_JSON=1", "schema-1", "not implemented", "doctor --policy-only", "2 ", "3 "} {
		if !strings.Contains(help, text) {
			t.Errorf("help lacks %q", text)
		}
	}
}
