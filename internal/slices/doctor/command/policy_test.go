package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func TestPolicyDiagnosticExitPrecedence(t *testing.T) {
	for _, tc := range []struct {
		issues []port.Issue
		exit   int
	}{
		{[]port.Issue{{Kind: port.Missing}}, 2},
		{[]port.Issue{{Kind: port.Type}, {Kind: port.Unreadable}}, 3},
		{[]port.Issue{{Kind: port.Unreadable}, {Kind: port.Type}}, 3},
		{[]port.Issue{{Kind: port.None}}, 70},
	} {
		var stdout, stderr bytes.Buffer
		ui := cli.UI{Stdout: &stdout, Stderr: &stderr}
		if code := presentPolicy(&ui, port.Result{Issues: tc.issues}, true); code != tc.exit {
			t.Fatalf("code %d", code)
		}
		var output struct {
			OK       bool
			Problems []policyProblem
			Policy   any
			Scope    string
			Schema   int
		}
		if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
			t.Fatal(err)
		}
		if output.OK || output.Policy != nil || output.Scope != "policy" || output.Schema != 1 || len(output.Problems) != len(tc.issues) {
			t.Fatalf("%#v", output)
		}
	}
}

func TestPolicyPlainFailureAndPointers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ui := cli.UI{Stdout: &stdout, Stderr: &stderr}
	code := presentPolicy(&ui, port.Result{Issues: []port.Issue{{Kind: port.UnknownKey, Path: []string{"a~/b", "1"}}}}, false)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "itos-container: itos-container.yaml/a~0~1b/1:") ||
		!strings.Contains(stderr.String(), "remove fields") {
		t.Fatalf("%d %q %q", code, stdout.String(), stderr.String())
	}
}

func TestPolicyDiagnosticEscapesOnlyPresentation(t *testing.T) {
	for _, tc := range []struct {
		name, key, pointer, location string
	}{
		{"newline", "bad\nkey", "/bad\nkey", `itos-container.yaml/bad\nkey`},
		{"literal escape", `bad\nkey`, `/bad\nkey`, `itos-container.yaml/bad\\nkey`},
		{"terminal control", "\x1b[2J\r\x00\x7f", "/\x1b[2J\r\x00\x7f", `itos-container.yaml/\x1b[2J\r\x00\x7f`},
		{"Unicode and pointer", "ñ~/\u0085\u2028\u2029", "/ñ~0~1\u0085\u2028\u2029", `itos-container.yaml/ñ~0~1\u0085\u2028\u2029`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			ui := cli.UI{Stdout: &stdout, Stderr: &stderr}
			result := port.Result{Issues: []port.Issue{{Kind: port.UnknownKey, Path: []string{tc.key}}}}
			if code := presentPolicy(&ui, result, true); code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
			want := "itos-container: " + tc.location + ": Field is not supported; remove fields absent from the version 1 schema\n"
			if stderr.String() != want {
				t.Fatalf("stderr %q, want %q", stderr.String(), want)
			}
			var output struct{ Problems []policyProblem }
			if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
				t.Fatal(err)
			}
			if len(output.Problems) != 1 || output.Problems[0].Path != tc.pointer {
				t.Fatalf("problems %#v, want path %q", output.Problems, tc.pointer)
			}
		})
	}
}
